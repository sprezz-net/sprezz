package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"
	"time"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/pkg/cryptoutil"

	"github.com/google/uuid"
)

var ErrDropAction = errors.New("drop action gracefully")

type ActivityServiceConfig struct {
	MaxActivitySizeBytes  int64
	EnableContextBackfill bool
	FollowersSyncCache    port.FollowersSyncCache
}

type ActivityService struct {
	storage               port.StoragePort
	mediaStorage          port.MediaStoragePort
	parser                port.JSONLDParserPort
	forwarder             port.OutboundDispatcher
	fetcher               port.RemoteFetcher
	syncCache             port.FollowersSyncCache
	maxActivitySizeBytes  int64
	enableContextBackfill bool
}

func NewActivityService(storage port.StoragePort, parser port.JSONLDParserPort, media port.MediaStoragePort, fetcher port.RemoteFetcher, cfg ActivityServiceConfig, forwarders ...port.OutboundDispatcher) *ActivityService {
	maxLimit := cfg.MaxActivitySizeBytes
	if maxLimit <= 0 {
		maxLimit = 102400 // 100KB secure fallback default
	}
	service := &ActivityService{
		storage:               storage,
		mediaStorage:          media,
		parser:                parser,
		fetcher:               fetcher,
		syncCache:             cfg.FollowersSyncCache,
		maxActivitySizeBytes:  maxLimit,
		enableContextBackfill: cfg.EnableContextBackfill,
	}
	if len(forwarders) > 0 {
		service.forwarder = forwarders[0]
	}
	return service
}

var _ port.ActivityServicePort = (*ActivityService)(nil)

func (s *ActivityService) handleLocalGroupActivity(ctx context.Context, task model.InboundTask, activityType string, actorVal interface{}) (bool, error) {
	if !s.isLocalActorGroup(ctx, task.ObjectIRI) {
		return false, nil
	}
	actorIRI := parseStringOrID(actorVal)
	if err := s.processGroupActivity(ctx, task, activityType, actorIRI, task.ObjectIRI); err != nil {
		return false, err
	}
	isJoinOrLeave := activityType == model.ShortJoin || activityType == model.ShortLeave
	return isJoinOrLeave, nil
}

// ProcessInboundTask handles incoming standard (non-media) ActivityPub payloads
func (s *ActivityService) ProcessInboundTask(ctx context.Context, task model.InboundTask) error {
	if s.maxActivitySizeBytes > 0 && int64(len(task.Payload)) > s.maxActivitySizeBytes {
		return fmt.Errorf("rejected inbound activity: payload size (%d bytes) exceeds maximum limit (%d bytes)", len(task.Payload), s.maxActivitySizeBytes)
	}

	var activity struct {
		Type   string      `json:"type"`
		Actor  interface{} `json:"actor"`
		Object interface{} `json:"object"`
	}
	_ = json.Unmarshal(task.Payload, &activity)

	isIdempotent, err := s.handleIdempotentDelete(ctx, activity.Type, activity.Object)
	if err != nil {
		return err
	}
	if isIdempotent {
		return nil
	}

	if err := s.validateInboundActivity(ctx, task.ActivityIRI, task.Payload); err != nil {
		if errors.Is(err, ErrDropAction) {
			return nil
		}
		return fmt.Errorf("side-effect mutation rejected: %w", err)
	}

	if activity.Type == model.ShortDelete {
		return s.processDeleteActivity(ctx, task, activity.Object)
	}

	if _, err := s.saveInboundActivityQuads(ctx, task); err != nil {
		return err
	}

	isGroupJoinOrLeave, err := s.handleLocalGroupActivity(ctx, task, activity.Type, activity.Actor)
	if err != nil {
		return err
	}
	if isGroupJoinOrLeave {
		return nil
	}

	if err := s.deliverAndForwardInbound(ctx, task); err != nil {
		return err
	}

	if s.enableContextBackfill {
		go func() {
			_ = s.maybeTriggerContextBackfill(context.Background(), task)
		}()
	}

	return nil
}

func (s *ActivityService) maybeTriggerContextBackfill(ctx context.Context, task model.InboundTask) error {
	if task.ObjectIRI == "" {
		return nil
	}

	quads, err := s.storage.StreamQuadsBySubject(ctx, task.ObjectIRI)
	if err != nil || len(quads) == 0 {
		return nil
	}

	var contextHistoryIRI string
	var contextIRI string
	for _, q := range quads {
		if q.Predicate == model.PredicateContextHistory {
			contextHistoryIRI = q.Object
		}
		if q.Predicate == model.PredicateContext {
			contextIRI = q.Object
		}
	}

	var targetIRI string
	if contextHistoryIRI != "" {
		targetIRI = contextHistoryIRI
	} else if contextIRI != "" {
		targetIRI = contextIRI
	}

	if targetIRI == "" {
		return nil
	}

	domain := extractDomain(targetIRI)
	localDomain := extractDomain(task.ObjectIRI)
	if domain == "" || domain == localDomain {
		return nil
	}

	items, err := s.storage.GetObjectsByContext(ctx, targetIRI)
	if err == nil && len(items) > 1 {
		return nil
	}

	return s.backfillRemoteContext(ctx, targetIRI, task.ObjectIRI)
}

func (s *ActivityService) backfillRemoteContext(ctx context.Context, contextIRI, targetIRI string) error {
	domain := extractDomain(targetIRI)
	if domain == "" {
		return nil
	}
	tenantID, err := s.storage.GetTenantIDByDomain(ctx, domain)
	if err != nil {
		return err
	}
	serverIRI, keys, err := s.storage.GetActorCredentials(ctx, tenantID, "server")
	if err != nil {
		return err
	}

	payload, err := s.fetcher.FetchSigned(ctx, contextIRI, serverIRI+model.SuffixMainKey, keys.PrivateKeyRSAPEM, keys.PrivateKeyEd25519PEM)
	if err != nil {
		return err
	}

	var collection struct {
		OrderedItems []interface{} `json:"orderedItems"`
		Items        []interface{} `json:"items"`
	}
	if err := json.Unmarshal(payload, &collection); err != nil {
		return err
	}

	items := collection.OrderedItems
	if len(items) == 0 {
		items = collection.Items
	}

	for _, item := range items {
		s.ingestContextItem(ctx, tenantID, item)
	}

	return nil
}

func (s *ActivityService) ingestContextItem(ctx context.Context, tenantID int32, item interface{}) {
	var itemIRI string
	var itemPayload []byte

	switch v := item.(type) {
	case string:
		itemIRI = v
	case map[string]interface{}:
		if id, ok := v["id"].(string); ok {
			itemIRI = id
			itemPayload, _ = json.Marshal(v)
		}
	}

	if itemIRI == "" {
		return
	}

	existing, _ := s.storage.GetLatestPayload(ctx, itemIRI)
	if len(existing) > 0 {
		return
	}

	if len(itemPayload) == 0 {
		serverIRI, keys, err := s.storage.GetActorCredentials(ctx, tenantID, "server")
		if err != nil {
			return
		}
		itemPayload, err = s.fetcher.FetchSigned(ctx, itemIRI, serverIRI+model.SuffixMainKey, keys.PrivateKeyRSAPEM, keys.PrivateKeyEd25519PEM)
		if err != nil {
			return
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return
	}

	_ = s.storage.EnqueueInbound(ctx, id.String(), itemIRI, itemIRI, tenantID, itemPayload)
}

// ProcessInboundMediaTask pipelines a media stream to MinIO and links it transactionally to the graph metadata.
func (s *ActivityService) ProcessInboundMediaTask(ctx context.Context, mediaCtx port.InboundMediaContext, task model.InboundTask) (port.MediaAttachmentInfo, error) {
	if s.mediaStorage == nil {
		return port.MediaAttachmentInfo{}, fmt.Errorf("media storage engine driver is not configured")
	}

	_, err := s.checkMediaQuota(ctx, mediaCtx.TenantID, mediaCtx.Size)
	if err != nil {
		return port.MediaAttachmentInfo{}, err
	}

	var width, height int
	if seeker, ok := mediaCtx.MediaStream.(io.ReadSeeker); ok {
		if strings.HasPrefix(mediaCtx.ContentType, "image/") {
			if cfg, _, err := image.DecodeConfig(seeker); err == nil {
				width = cfg.Width
				height = cfg.Height
			}
			_, _ = seeker.Seek(0, io.SeekStart) // Rewind stream!
		}
	}

	stableKey, sha256Hex, err := s.mediaStorage.PutObject(ctx, mediaCtx.ObjectName, mediaCtx.MediaStream, mediaCtx.ContentType)
	if err != nil {
		return port.MediaAttachmentInfo{}, fmt.Errorf("media workflow aborted due to storage upload failure: %w", err)
	}

	quads, err := s.parser.ToQuads(ctx, 0, task.ObjectIRI, task.Payload)
	if err != nil {
		_ = s.mediaStorage.DeleteObject(ctx, stableKey)
		return port.MediaAttachmentInfo{}, fmt.Errorf("failed to parse activity payload to quads during media task: %w", err)
	}

	if writer, ok := s.storage.(port.GraphVersionWriter); ok {
		err := writer.SaveGraphVersionWithMedia(ctx, port.MediaAttachmentParams{
			ObjectName:   stableKey,
			OriginalName: mediaCtx.OriginalName,
			SHA256Hex:    sha256Hex,
			ContentType:  mediaCtx.ContentType,
			FileSize:     mediaCtx.Size,
			TenantID:     mediaCtx.TenantID,
			ActorIRI:     mediaCtx.ActorIRI,
			ActivityIRI:  task.ActivityIRI,
			ObjectIRI:    task.ObjectIRI,
			Payload:      task.Payload,
			Quads:        quads,
			Width:        width,
			Height:       height,
		})
		if err != nil {
			_ = s.mediaStorage.DeleteObject(ctx, stableKey)
			return port.MediaAttachmentInfo{}, fmt.Errorf("failed to commit graph and media attachment relationships: %w", err)
		}
		digest, _ := cryptoutil.ToDigestMultibaseFromHex(sha256Hex)
		return port.MediaAttachmentInfo{
			ObjectName:      stableKey,
			OriginalName:    mediaCtx.OriginalName,
			SHA256Hex:       sha256Hex,
			DigestMultibase: digest,
			ContentType:     mediaCtx.ContentType,
			Size:            mediaCtx.Size,
			Width:           width,
			Height:          height,
		}, nil
	}

	_ = s.mediaStorage.DeleteObject(ctx, stableKey)
	return port.MediaAttachmentInfo{}, fmt.Errorf("storage port does not implement required GraphVersionWriter extension")
}

// PurgeOrphanedMedia provides the domain coordination step for clearing stranded files.
// It directly drives the media storage port while applying strict structural masking to standard system logging.
func (s *ActivityService) PurgeOrphanedMedia(ctx context.Context, tempObjectKey string) error {
	if s.mediaStorage == nil || tempObjectKey == "" {
		return nil
	}

	// 1. Drop the physical binary asset chunk from infrastructure
	_ = s.mediaStorage.DeleteObject(ctx, tempObjectKey)

	// 2. Clear out the database weights directly to instantly restore tenant limits
	if s.storage != nil {
		_ = s.storage.RemoveMediaRecord(ctx, tempObjectKey)
	}

	return nil
}

// DispatchOutboundActivity routes activities outbound, consolidating deliveries using sharedInboxes
func (s *ActivityService) DispatchOutboundActivity(ctx context.Context, activityIRI string, actorIRI string, payload []byte) error {
	if s.forwarder == nil {
		return fmt.Errorf("outbound dispatcher is not configured")
	}

	targetsMap, err := extractAddressingTargets(payload)
	if err != nil {
		return err
	}

	s.expandFollowers(ctx, targetsMap)

	dualKeys, err := s.storage.GetActorDualKeys(ctx, actorIRI)
	if err != nil {
		return fmt.Errorf("load actor dual-key credentials: %w", err)
	}

	targetKeyID := actorIRI + model.SuffixMainKey

	inboxesMap, err := s.resolveOutboundInboxes(ctx, actorIRI, targetsMap, payload, activityIRI)
	if err != nil {
		return err
	}

	var lastErr error
	for inbox := range inboxesMap {
		inboxDomain := extractDomain(inbox)
		followers, _ := s.GetFollowersTimeline(ctx, actorIRI, 10000, 0)
		digest := ComputeFollowersDigest(followers, inboxDomain)

		deliveryCtx := ctx
		if digest != "" {
			syncURL := actorIRI + "/followers_synchronization"
			collectionID := actorIRI + "/followers"
			headerVal := fmt.Sprintf("collectionId=%q,url=%q,digest=%q", collectionID, syncURL, digest)
			deliveryCtx = context.WithValue(ctx, model.CollectionSyncHeaderKey, headerVal)
		}

		err = s.forwarder.ForwardFederatedActivity(
			deliveryCtx,
			inbox,
			targetKeyID,
			dualKeys.PrivateKeyRSAPEM,
			dualKeys.PrivateKeyEd25519PEM,
			payload,
		)
		if err != nil {
			lastErr = err
		}
	}

	if lastErr != nil {
		return fmt.Errorf("dispatch failure: %w", lastErr)
	}

	return nil
}

func (s *ActivityService) GetFollowersTimeline(ctx context.Context, actorIRI string, limit, offset int) ([]string, error) {
	quads, err := s.storage.StreamQuadsBySubject(ctx, actorIRI)
	if err != nil {
		return nil, fmt.Errorf("failed to stream quads for actor %s: %w", actorIRI, err)
	}

	followers := make([]string, 0)
	for _, q := range quads {
		if q.Predicate == model.PredicateFollower || q.Predicate == model.PredicateFollowers {
			followers = append(followers, q.Object)
		}
	}

	if offset >= len(followers) {
		return []string{}, nil
	}
	end := offset + limit
	if end > len(followers) {
		end = len(followers)
	}
	return followers[offset:end], nil
}

// FilterPublicAndAuthorizedQuads evaluates an array of quads and removes any activities
// that do not match public audience addresses or explicit reader authorizations.
func (s *ActivityService) FilterPublicAndAuthorizedQuads(ctx context.Context, readerActorIRI string, quads []model.Quad) []model.Quad {
	if len(quads) == 0 {
		return quads
	}

	// Group quads by their GraphID to evaluate security boundaries per activity version
	graphs := make(map[int64][]model.Quad)
	for _, q := range quads {
		graphs[q.GraphID] = append(graphs[q.GraphID], q)
	}

	authorizedGraphIDs := make(map[int64]struct{})
	for graphID, graphQuads := range graphs {
		if s.isGraphAuthorized(graphID, readerActorIRI, graphQuads) {
			authorizedGraphIDs[graphID] = struct{}{}
		}
	}

	// Filter out quads belonging to unauthorized graph versions before pagination occurs
	filtered := make([]model.Quad, 0, len(quads))
	for _, q := range quads {
		if _, authorized := authorizedGraphIDs[q.GraphID]; authorized {
			filtered = append(filtered, q)
		}
	}

	return filtered
}

// isGraphAuthorized iterates a single graph's quads to determine public or direct visibility.
func (s *ActivityService) isGraphAuthorized(graphID int64, readerActorIRI string, quads []model.Quad) bool {
	isPublic := false
	isDirectRecipient := false

	for _, q := range quads {
		if q.GraphID != graphID {
			continue
		}

		if isAddressingPredicate(q.Predicate) {
			cleanObject := strings.Trim(q.Object, `"'`)
			if cleanObject == "https://www.w3.org/ns/activitystreams#Public" {
				isPublic = true
			}
			if readerActorIRI != "" && cleanObject == readerActorIRI {
				isDirectRecipient = true
			}
		}
	}

	return isPublic || isDirectRecipient
}

// GetCollectionTimeline retrieves an actor's collection (e.g. "inbox" or "outbox"),
// parses payloads into intermediate quads, applies case-insensitive privacy-aware audience checks,
// and streams down a safe, filtered set of authorized payload slices.
func (s *ActivityService) GetCollectionTimeline(ctx context.Context, readerActorIRI string, actorIRI string, collection string, limit, offset int) ([][]byte, error) {
	// Special Case: pendingFollowers, pendingFollowing, blocked, and blocks are only readable by the owner of the collection (readerActorIRI == actorIRI)
	if model.IsPrivateCollection(collection) {
		if readerActorIRI == "" || readerActorIRI != actorIRI {
			return [][]byte{}, nil
		}
		return s.storage.GetCollectionPayloads(ctx, actorIRI, collection, limit, offset)
	}

	// 1. Stream the raw candidate payload entries directly out of your postgres storage engine port
	rawPayloads, err := s.storage.GetCollectionPayloads(ctx, actorIRI, collection, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to stream candidate collection payloads: %w", err)
	}

	if len(rawPayloads) == 0 {
		return rawPayloads, nil
	}

	authorizedPayloads := make([][]byte, 0, len(rawPayloads))

	// 2. Iterate through each activity document version to evaluate security contexts
	for i, payload := range rawPayloads {
		// Assign a temporary synthetic GraphID mapping identifier to group quads locally
		syntheticGraphID := int64(i + 1)

		// Expand the raw JSON-LD structure into absolute RDF quad matrices
		quads, err := s.parser.ToQuads(ctx, syntheticGraphID, actorIRI, payload)
		if err != nil {
			// Skip malformed entries gracefully without failing the entire timeline query pipeline
			continue
		}

		// Run the quads through your case-insensitive audience validation engine
		filteredQuads := s.FilterPublicAndAuthorizedQuads(ctx, readerActorIRI, quads)

		// If the returned quad pool is not empty, the reader has explicit permission to read this event
		if len(filteredQuads) > 0 {
			authorizedPayloads = append(authorizedPayloads, payload)
		}
	}

	return authorizedPayloads, nil
}

func (s *ActivityService) RotateLocalActorKeys(ctx context.Context, tenantID int32, username string) (string, error) {
	actorIRI, dualKeys, err := s.storage.GetActorCredentials(ctx, tenantID, username)
	if err != nil {
		return "", fmt.Errorf("rotation aborted: failed to locate existing actor: %w", err)
	}

	now := time.Now().UTC()
	validFrom := now.Add(-24 * time.Hour)

	// Archive steps remain compact
	rsaPubKeyPEM, err := cryptoutil.ExtractRSAPublicKey(dualKeys.PrivateKeyRSAPEM)
	if err == nil {
		_ = s.storage.ArchiveKeyHistory(ctx, actorIRI, "RSA", rsaPubKeyPEM, validFrom, now)
	}

	if dualKeys.PrivateKeyEd25519PEM != "" {
		edPubKeyPEM, err := cryptoutil.ExtractEd25519PublicKey(dualKeys.PrivateKeyEd25519PEM)
		if err == nil {
			_ = s.storage.ArchiveKeyHistory(ctx, actorIRI, "Ed25519", edPubKeyPEM, validFrom, now)
		}
	}

	// Reuse the identical centralized key minting function
	newKeys, err := cryptoutil.MintNewKeyPair()
	if err != nil {
		return "", fmt.Errorf("failed to mint fresh keys during rotation: %w", err)
	}

	// Overwrite the row, discarding old private keys from memory
	err = s.storage.CreateActorCredential(ctx, actorIRI, tenantID, username, newKeys.RSAPrivatePEM, newKeys.Ed25519PrivatePEM)
	if err != nil {
		return "", fmt.Errorf("failed to overwrite current local credentials: %w", err)
	}

	return actorIRI, nil
}

func (s *ActivityService) AcceptFollow(ctx context.Context, followedActorIRI, followActivityIRI string) error {
	return s.handleFollowResponse(ctx, followedActorIRI, followActivityIRI, true)
}

func (s *ActivityService) RejectFollow(ctx context.Context, followedActorIRI, followActivityIRI string) error {
	return s.handleFollowResponse(ctx, followedActorIRI, followActivityIRI, false)
}

func (s *ActivityService) isLocalActorGroup(ctx context.Context, actorIRI string) bool {
	// A group can only be processed locally if it is a local actor (i.e. has local credentials)
	if _, err := s.storage.GetActorDualKeys(ctx, actorIRI); err != nil {
		return false
	}
	quads, err := s.storage.StreamQuadsBySubject(ctx, actorIRI)
	if err != nil || len(quads) == 0 {
		return false
	}
	for _, q := range quads {
		if q.Predicate == model.RDFType && q.Object == model.ActorGroup {
			return true
		}
	}
	return false
}

func (s *ActivityService) isGroupMember(ctx context.Context, senderIRI, groupIRI string) (bool, error) {
	quads, err := s.storage.StreamQuadsBySubject(ctx, groupIRI)
	if err != nil {
		return false, err
	}
	for _, q := range quads {
		if q.Predicate == model.PredicateFollower && q.Object == senderIRI {
			return true, nil
		}
	}
	return false, nil
}

func (s *ActivityService) processGroupActivity(ctx context.Context, task model.InboundTask, actType, senderIRI, groupIRI string) error {
	switch actType {
	case model.ShortJoin:
		return s.handleGroupJoin(ctx, senderIRI, groupIRI)
	case model.ShortLeave:
		return s.handleGroupLeave(ctx, senderIRI, groupIRI)
	default:
		isMember, err := s.isGroupMember(ctx, senderIRI, groupIRI)
		if err != nil {
			return err
		}
		if !isMember {
			return fmt.Errorf("rejected group activity: sender %s is not a member of group %s", senderIRI, groupIRI)
		}
		return s.announceGroupActivity(ctx, task, groupIRI)
	}
}

func (s *ActivityService) handleGroupJoin(ctx context.Context, senderIRI, groupIRI string) error {
	followerQuads := []model.Quad{
		{
			Subject:   groupIRI,
			Predicate: model.PredicateFollower,
			Object:    senderIRI,
			ObjType:   model.NamedNode,
		},
	}
	if err := s.storage.SaveQuads(ctx, followerQuads); err != nil {
		return fmt.Errorf("failed to save group follower relationship: %w", err)
	}

	s.evictFollowersDigest(ctx, groupIRI, extractDomain(senderIRI))

	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	domain := extractDomain(groupIRI)
	activityIRI := fmt.Sprintf("https://%s/activity/%s", domain, id.String())

	acceptActivity := map[string]interface{}{
		model.JSONLDContext: model.ContextActivityStreams,
		"id":                activityIRI,
		"type":              model.ShortAccept,
		"actor":             groupIRI,
		"object": map[string]interface{}{
			"type":   model.ShortJoin,
			"actor":  senderIRI,
			"object": groupIRI,
		},
		"to": []string{senderIRI},
	}
	payload, err := json.Marshal(acceptActivity)
	if err != nil {
		return err
	}

	return s.DispatchOutboundActivity(ctx, activityIRI, groupIRI, payload)
}

func (s *ActivityService) handleGroupLeave(ctx context.Context, senderIRI, groupIRI string) error {
	if err := s.storage.RemoveQuadEdge(ctx, groupIRI, model.PredicateFollower, senderIRI); err != nil {
		return fmt.Errorf("failed to remove group follower relationship: %w", err)
	}

	s.evictFollowersDigest(ctx, groupIRI, extractDomain(senderIRI))

	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	domain := extractDomain(groupIRI)
	activityIRI := fmt.Sprintf("https://%s/activity/%s", domain, id.String())

	acceptActivity := map[string]interface{}{
		model.JSONLDContext: model.ContextActivityStreams,
		"id":                activityIRI,
		"type":              model.ShortAccept,
		"actor":             groupIRI,
		"object": map[string]interface{}{
			"type":   model.ShortLeave,
			"actor":  senderIRI,
			"object": groupIRI,
		},
		"to": []string{senderIRI},
	}
	payload, err := json.Marshal(acceptActivity)
	if err != nil {
		return err
	}

	return s.DispatchOutboundActivity(ctx, activityIRI, groupIRI, payload)
}

func (s *ActivityService) announceGroupActivity(ctx context.Context, task model.InboundTask, groupIRI string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	domain := extractDomain(groupIRI)
	activityIRI := fmt.Sprintf("https://%s/activity/%s", domain, id.String())

	announceActivity := map[string]interface{}{
		model.JSONLDContext: model.ContextActivityStreams,
		"id":                activityIRI,
		"type":              model.ShortAnnounce,
		"actor":             groupIRI,
		"object":            task.ActivityIRI,
		"to":                []string{groupIRI + "/followers"},
	}
	payload, err := json.Marshal(announceActivity)
	if err != nil {
		return err
	}

	return s.DispatchOutboundActivity(ctx, activityIRI, groupIRI, payload)
}
