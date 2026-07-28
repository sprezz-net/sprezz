package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
)

type ActivityService struct {
	storage      port.StoragePort
	mediaStorage port.MediaStoragePort
	parser       port.JSONLDParserPort
	forwarder    port.OutboundDispatcher
}

func NewActivityService(storage port.StoragePort, parser port.JSONLDParserPort, media port.MediaStoragePort, forwarders ...port.OutboundDispatcher) *ActivityService {
	service := &ActivityService{
		storage:      storage,
		mediaStorage: media,
		parser:       parser,
	}
	if len(forwarders) > 0 {
		service.forwarder = forwarders[0]
	}
	return service
}

var _ port.ActivityServicePort = (*ActivityService)(nil)

// ProcessInboundTask handles incoming standard (non-media) ActivityPub payloads
func (s *ActivityService) ProcessInboundTask(ctx context.Context, task model.InboundTask) error {
	var quads []model.Quad
	var err error

	// 1. Parse activity payload into RDF quads
	if writer, ok := s.storage.(port.GraphVersionWriter); ok {
		quads, err = s.parser.ToQuads(ctx, 0, task.ObjectIRI, task.Payload)
		if err != nil {
			return fmt.Errorf("failed to parse activity payload to quads: %w", err)
		}
		if err := writer.SaveGraphVersion(ctx, task.ActivityIRI, task.ObjectIRI, task.Payload, quads); err != nil {
			return fmt.Errorf("failed to save graph version and quads: %w", err)
		}
	} else {
		// Fallback path utilizing explicit graph versioning combined with the optimized ports layer
		graphID, err := s.storage.CreateGraphVersion(ctx, task.ActivityIRI, task.ObjectIRI, task.Payload)
		if err != nil {
			return fmt.Errorf("failed to create graph version: %w", err)
		}

		quads, err = s.parser.ToQuads(ctx, graphID, task.ObjectIRI, task.Payload)
		if err != nil {
			return fmt.Errorf("failed to parse activity payload to quads: %w", err)
		}

		if err := s.storage.SaveQuads(ctx, quads); err != nil {
			return fmt.Errorf("failed to save quads: %w", err)
		}
	}

	// 2. Perform spec-compliant shared and direct inbox delivery mapping.
	// We extract the addressing targets (to, cc, bto, bcc, audience) of the activity JSON payload.
	// Since direct inbox URLs can be *any* path and are completely decoupled from Chi parameters,
	// checking the target recipient actors' registered IRIs against target addressing properties
	// is the universally correct implementation for both direct and shared inbox.
	var envelope struct {
		To       interface{} `json:"to"`
		Cc       interface{} `json:"cc"`
		Bto      interface{} `json:"bto"`
		Bcc      interface{} `json:"bcc"`
		Audience interface{} `json:"audience"`
		Actor    string      `json:"actor"`
	}
	_ = json.Unmarshal(task.Payload, &envelope)

	// Collect all targets mentioned in addressing fields
	targetsMap := make(map[string]struct{})
	collectAddresses := func(val interface{}) {
		if val == nil {
			return
		}
		switch v := val.(type) {
		case string:
			targetsMap[v] = struct{}{}
		case []interface{}:
			for _, item := range v {
				if str, ok := item.(string); ok {
					targetsMap[str] = struct{}{}
				}
			}
		}
	}
	collectAddresses(envelope.To)
	collectAddresses(envelope.Cc)
	collectAddresses(envelope.Bto)
	collectAddresses(envelope.Bcc)
	collectAddresses(envelope.Audience)

	// Also check if the task.ObjectIRI itself is a target. In standard direct inbox deliveries,
	// the requested direct inbox URL matches a local actor's configured inbox collection.
	// We can check if task.ObjectIRI is a local actor profile and append it to our targets map.
	if task.ObjectIRI != "" {
		targetsMap[task.ObjectIRI] = struct{}{}
	}

	// Deliver to matching local targets
	for target := range targetsMap {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		// Check if target is a direct local actor IRI
		_, err := s.storage.GetActorDualKeys(ctx, target)
		if err == nil {
			// Deliver to this local actor's inbox!
			_ = s.storage.RecordActorInboxDelivery(ctx, target, task.ActivityIRI)
			continue
		}

		// Check if target is a followers collection of a local actor (e.g. target ends with "/followers")
		if strings.HasSuffix(target, "/followers") {
			localActorIRI := strings.TrimSuffix(target, "/followers")
			_, err := s.storage.GetActorDualKeys(ctx, localActorIRI)
			if err == nil {
				// This is a local actor's followers collection.
				// We need to deliver to all local followers of this actor.
				followers, err := s.GetFollowersTimeline(ctx, localActorIRI, 1000, 0)
				if err == nil {
					for _, follower := range followers {
						// Double-check if the follower is a local actor on our system
						_, err := s.storage.GetActorDualKeys(ctx, follower)
						if err == nil {
							_ = s.storage.RecordActorInboxDelivery(ctx, follower, task.ActivityIRI)
						}
					}
				}
			}
		}
	}

	return nil
}

// ProcessInboundMediaTask pipelines a media stream to MinIO and links it transactionally to the graph metadata.
func (s *ActivityService) ProcessInboundMediaTask(ctx context.Context, mediaCtx port.InboundMediaContext, task model.InboundTask) error {
	if s.mediaStorage == nil {
		return fmt.Errorf("media storage engine driver is not configured")
	}

	// 1. Resolve tenant identity dynamically from context strings
	var resolvedTenantID int32 = 1 // Safe baseline default mapping
	if mediaCtx.TenantID != "" {
		if val, err := strconv.ParseInt(mediaCtx.TenantID, 10, 32); err == nil {
			resolvedTenantID = int32(val)
		}
	}

	// 2. Execute the Pre-Flight Quota Guard verification FIRST before any storage I/O
	hasQuota, err := s.storage.VerifyIncomingQuota(ctx, resolvedTenantID, mediaCtx.Size)
	if err != nil {
		return fmt.Errorf("quota audit system interception error: %w", err)
	}
	if !hasQuota {
		// Terminate execution paths instantly before invoking any remote connection sockets
		return fmt.Errorf("media workflow aborted: storage authorization ceiling threshold exceeded")
	}

	// 3. Stream the object payload to MinIO only AFTER quota validation passes successfully.
	stableKey, sha256Hex, err := s.mediaStorage.PutObject(ctx, mediaCtx.ObjectName, mediaCtx.MediaStream, mediaCtx.ContentType)
	if err != nil {
		return fmt.Errorf("media workflow aborted due to storage upload failure: %w", err)
	}

	// 4. Parse the JSON-LD payload into quads before triggering the database routine
	quads, err := s.parser.ToQuads(ctx, 0, task.ObjectIRI, task.Payload)
	if err != nil {
		_ = s.mediaStorage.DeleteObject(ctx, stableKey) // Compensating removal
		return fmt.Errorf("failed to parse activity payload to quads during media task: %w", err)
	}

	// 5. Delegate atomic multi-table execution down to the transaction writer engine
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
		})
		if err != nil {
			_ = s.mediaStorage.DeleteObject(ctx, stableKey)
			return fmt.Errorf("failed to commit graph and media attachment relationships: %w", err)
		}
		return nil
	}

	_ = s.mediaStorage.DeleteObject(ctx, stableKey)
	return fmt.Errorf("storage port does not implement required GraphVersionWriter extension")
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

func (s *ActivityService) DispatchOutboundActivity(ctx context.Context, activityIRI string, actorIRI string, payload []byte) error {
	if s.forwarder == nil {
		return fmt.Errorf("outbound dispatcher is not configured")
	}
	var envelope struct {
		Inbox string `json:"inbox"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("decode outbound activity: %w", err)
	}
	if envelope.Inbox == "" {
		return fmt.Errorf("outbound activity %s has no target inbox", activityIRI)
	}

	dualKeys, err := s.storage.GetActorDualKeys(ctx, actorIRI)
	if err != nil {
		return fmt.Errorf("load actor dual-key credentials: %w", err)
	}

	targetKeyID := actorIRI + "#main-key"

	return s.forwarder.ForwardFederatedActivity(
		ctx,
		envelope.Inbox,
		targetKeyID,
		dualKeys.PrivateKeyRSAPEM,
		dualKeys.PrivateKeyEd25519PEM,
		payload,
	)
}

func (s *ActivityService) GetFollowersTimeline(ctx context.Context, actorIRI string, limit, offset int) ([]string, error) {
	quads, err := s.storage.StreamQuadsBySubject(ctx, actorIRI)
	if err != nil {
		return nil, fmt.Errorf("failed to stream quads for actor %s: %w", actorIRI, err)
	}

	followers := make([]string, 0)
	for _, q := range quads {
		// Using strings.Contains to catch any W3C protocol layout
		// variations variation dynamically (handles both http/https and singular/plural specs).
		if strings.Contains(q.Predicate, "activitystreams#follower") || strings.Contains(q.Predicate, "activitystreams#followers") {
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

// isAddressingPredicate isolates the specific W3C target addressing field checks.
func isAddressingPredicate(predicate string) bool {
	// Standardized on lowercase containment substrings to match absolute URLs
	// featuring both hash fragments or folder path separators emitted by the parser engine.
	addressingTerms := []string{"activitystreams#to", "activitystreams#cc", "activitystreams#bto", "activitystreams#bcc", "activitystreams#audience"}
	for _, term := range addressingTerms {
		if strings.Contains(strings.ToLower(predicate), term) {
			return true
		}
	}
	return false
}

// GetCollectionTimeline retrieves an actor's collection (e.g. "inbox" or "outbox"),
// parses payloads into intermediate quads, applies case-insensitive privacy-aware audience checks,
// and streams down a safe, filtered set of authorized payload slices.
func (s *ActivityService) GetCollectionTimeline(ctx context.Context, readerActorIRI string, actorIRI string, collection string, limit, offset int) ([][]byte, error) {
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
	rsaPubKeyPEM, err := model.ExtractRSAPublicKey(dualKeys.PrivateKeyRSAPEM)
	if err == nil {
		_ = s.storage.ArchiveKeyHistory(ctx, actorIRI, "RSA", rsaPubKeyPEM, validFrom, now)
	}

	if dualKeys.PrivateKeyEd25519PEM != "" {
		edPubKeyPEM, err := model.ExtractEd25519PublicKey(dualKeys.PrivateKeyEd25519PEM)
		if err == nil {
			_ = s.storage.ArchiveKeyHistory(ctx, actorIRI, "Ed25519", edPubKeyPEM, validFrom, now)
		}
	}

	// Reuse the identical centralized key minting function
	newKeys, err := model.MintNewKeyPair()
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
