package service

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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

// extractAddressingTargets pulls addressing target IRIs from the activity JSON payload
func extractAddressingTargets(payload []byte) (map[string]struct{}, error) {
	var envelope struct {
		To       interface{} `json:"to"`
		Cc       interface{} `json:"cc"`
		Bto      interface{} `json:"bto"`
		Bcc      interface{} `json:"bcc"`
		Audience interface{} `json:"audience"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode addressing targets: %w", err)
	}

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

	return targetsMap, nil
}

// resolveActorInbox finds the sharedInbox or fallback direct inbox for a remote actor
func (s *ActivityService) resolveActorInbox(ctx context.Context, actorIRI string) (string, error) {
	quads, err := s.storage.StreamQuadsBySubject(ctx, actorIRI)
	if err != nil {
		return "", err
	}
	var resolvedInbox string
	var sharedInbox string

	for _, q := range quads {
		pred := strings.ToLower(q.Predicate)
		if strings.Contains(pred, "sharedinbox") {
			sharedInbox = strings.Trim(q.Object, `"'`)
		}
		if strings.Contains(pred, "inbox") && !strings.Contains(pred, "sharedinbox") {
			resolvedInbox = strings.Trim(q.Object, `"'`)
		}
	}

	if sharedInbox != "" {
		return sharedInbox, nil
	}
	if resolvedInbox != "" {
		return resolvedInbox, nil
	}
	return "", fmt.Errorf("no inbox found for actor %s", actorIRI)
}

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
	targetsMap, err := extractAddressingTargets(task.Payload)
	if err != nil {
		return err
	}

	// Also check if the task.ObjectIRI itself is a target. In standard direct inbox deliveries,
	// the requested direct inbox URL matches a local actor's configured inbox collection.
	if task.ObjectIRI != "" {
		targetsMap[task.ObjectIRI] = struct{}{}
	}

	// Expand target followers
	s.expandFollowers(ctx, targetsMap)

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

// DispatchOutboundActivity routes activities outbound, consolidating deliveries using sharedInboxes
func (s *ActivityService) DispatchOutboundActivity(ctx context.Context, activityIRI string, actorIRI string, payload []byte) error {
	if s.forwarder == nil {
		return fmt.Errorf("outbound dispatcher is not configured")
	}

	// 1. Gather all unique targets from addressing fields
	targetsMap, err := extractAddressingTargets(payload)
	if err != nil {
		return err
	}

	// Expand target followers first
	s.expandFollowers(ctx, targetsMap)

	// 2. Load the sender's dual-key credentials
	dualKeys, err := s.storage.GetActorDualKeys(ctx, actorIRI)
	if err != nil {
		return fmt.Errorf("load actor dual-key credentials: %w", err)
	}

	targetKeyID := actorIRI + "#main-key"

	// 3. Resolve the target inboxes (preferring domain-level sharedInbox via FEP-d556, falling back to direct inbox)
	inboxesMap := make(map[string]struct{})

	// Group remote targets by domain using our generic helper
	domainToRecipients := s.groupRemoteTargetsByDomain(ctx, targetsMap)

	for domain, recipients := range domainToRecipients {
		// Attempt to discover the server-level shared inbox for this domain
		sharedInboxURL, err := s.resolveServerActorInbox(ctx, domain, actorIRI)
		if err == nil && sharedInboxURL != "" {
			inboxesMap[sharedInboxURL] = struct{}{}
		} else {
			// Fallback: resolve individual recipient inboxes if server actor discovery fails
			for _, target := range recipients {
				inboxURL, err := s.resolveActorInbox(ctx, target)
				if err == nil && inboxURL != "" {
					inboxesMap[inboxURL] = struct{}{}
				}
			}
		}
	}

	// 4. Fallback if no target inboxes were resolved but envelope.Inbox is specified
	if len(inboxesMap) == 0 {
		var envelope struct {
			Inbox string `json:"inbox"`
		}
		_ = json.Unmarshal(payload, &envelope)
		if envelope.Inbox == "" {
			return fmt.Errorf("outbound activity %s has no target inbox", activityIRI)
		}
		inboxesMap[envelope.Inbox] = struct{}{}
	}

	// 5. Dispatch the activity to each unique inbox endpoint
	var lastErr error
	for inbox := range inboxesMap {
		err = s.forwarder.ForwardFederatedActivity(
			ctx,
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

func extractDomain(iri string) string {
	u, err := url.Parse(iri)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// expandFollowers resolves and expands all /followers collections in targetsMap to direct recipient IRIs
func (s *ActivityService) expandFollowers(ctx context.Context, targetsMap map[string]struct{}) {
	var targets []string
	for target := range targetsMap {
		targets = append(targets, target)
	}

	for i := 0; i < len(targets); i++ {
		target := strings.TrimSpace(targets[i])
		if target == "" {
			continue
		}
		if strings.HasSuffix(target, "/followers") {
			localActorIRI := strings.TrimSuffix(target, "/followers")
			_, err := s.storage.GetActorDualKeys(ctx, localActorIRI)
			if err == nil {
				followers, err := s.GetFollowersTimeline(ctx, localActorIRI, 1000, 0)
				if err == nil {
					for _, follower := range followers {
						if _, exists := targetsMap[follower]; !exists {
							targetsMap[follower] = struct{}{}
							targets = append(targets, follower)
						}
					}
				}
			}
		}
	}
}

// groupRemoteTargetsByDomain filters out local actors and groups remote targets by their server domains
func (s *ActivityService) groupRemoteTargetsByDomain(ctx context.Context, targetsMap map[string]struct{}) map[string][]string {
	domainToRecipients := make(map[string][]string)
	for target := range targetsMap {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		// Skip if local actor
		_, err := s.storage.GetActorDualKeys(ctx, target)
		if err == nil {
			continue
		}

		domain := extractDomain(target)
		if domain != "" {
			domainToRecipients[domain] = append(domainToRecipients[domain], target)
		}
	}
	return domainToRecipients
}

func (s *ActivityService) resolveServerActorInbox(ctx context.Context, targetDomain string, senderActorIRI string) (string, error) {
	// 1. Try local cache first
	if cachedInbox := s.resolveCachedServerInbox(ctx, targetDomain); cachedInbox != "" {
		return cachedInbox, nil
	}

	// 2. Discover via signed FEP-d556 process (Signed with dual-keys)
	serverActorIRI, serverKeys, err := s.getLocalServerCredentials(ctx, senderActorIRI)
	if err != nil {
		return "", err
	}

	targetKeyID := serverActorIRI + "#main-key"

	// Step A: WebFinger query for resource=https://<domain>
	actorProfileURL, err := s.discoverRemoteActorIRI(ctx, targetDomain, targetKeyID, serverKeys.PrivateKeyRSAPEM, serverKeys.PrivateKeyEd25519PEM)
	if err != nil {
		return "", err
	}

	// Step B: Fetch the actor profile
	resolvedSharedInbox, profileBody, err := s.discoverRemoteSharedInbox(ctx, actorProfileURL, targetKeyID, serverKeys.PrivateKeyRSAPEM, serverKeys.PrivateKeyEd25519PEM)
	if err != nil {
		return "", err
	}

	// Cache the resolved inbox
	s.cacheServerInbox(ctx, targetDomain, resolvedSharedInbox, profileBody)

	return resolvedSharedInbox, nil
}

// resolveCachedServerInbox queries our database for a cached server actor's shared inbox URL
func (s *ActivityService) resolveCachedServerInbox(ctx context.Context, targetDomain string) string {
	inboxURL, err := s.resolveActorInbox(ctx, "https://"+targetDomain)
	if err == nil && inboxURL != "" {
		return inboxURL
	}
	inboxURL, err = s.resolveActorInbox(ctx, "http://"+targetDomain)
	if err == nil && inboxURL != "" {
		return inboxURL
	}
	return ""
}

// getLocalServerCredentials resolves the local tenant and system server credentials (RSA + Ed25519)
func (s *ActivityService) getLocalServerCredentials(ctx context.Context, senderActorIRI string) (string, *model.ActorDualKeys, error) {
	senderHost := extractDomain(senderActorIRI)
	if senderHost == "" {
		return "", nil, fmt.Errorf("invalid sender actor IRI format")
	}

	tenantID, err := s.storage.GetOrCreateTenantByDomain(ctx, senderHost)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get tenant ID: %w", err)
	}

	serverActorIRI, serverKeys, err := s.storage.GetActorCredentials(ctx, tenantID, "server")
	if err != nil {
		return "", nil, fmt.Errorf("failed to load local server actor credentials: %w", err)
	}

	return serverActorIRI, serverKeys, nil
}

// discoverRemoteActorIRI queries the WebFinger JRD of a remote domain to resolve its server-controlled actor IRI
func (s *ActivityService) discoverRemoteActorIRI(ctx context.Context, targetDomain, keyID, privateKeyRSAPEM, privateKeyEd25519PEM string) (string, error) {
	webfingerURL := fmt.Sprintf("https://%s/.well-known/webfinger?resource=https://%s", targetDomain, targetDomain)
	wfBody, err := s.signedGet(ctx, webfingerURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM)
	if err != nil {
		// Fallback to http if https fails
		webfingerURL = fmt.Sprintf("http://%s/.well-known/webfinger?resource=http://%s", targetDomain, targetDomain)
		wfBody, err = s.signedGet(ctx, webfingerURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM)
		if err != nil {
			return "", fmt.Errorf("webfinger discovery failed: %w", err)
		}
	}

	var wfResp struct {
		Links []struct {
			Rel  string `json:"rel"`
			Type string `json:"type"`
			Href string `json:"href"`
		} `json:"links"`
	}
	if err := json.Unmarshal(wfBody, &wfResp); err != nil {
		return "", fmt.Errorf("failed to parse webfinger response: %w", err)
	}

	for _, link := range wfResp.Links {
		if link.Rel == "self" && (strings.Contains(link.Type, "activity") || strings.Contains(link.Type, "json")) {
			return link.Href, nil
		}
	}

	return "", fmt.Errorf("no self link found in webfinger response")
}

// discoverRemoteSharedInbox fetches and parses a remote actor's JSON-LD profile to extract its shared inbox URL
func (s *ActivityService) discoverRemoteSharedInbox(ctx context.Context, actorProfileURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM string) (string, []byte, error) {
	profileBody, err := s.signedGet(ctx, actorProfileURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM)
	if err != nil {
		return "", nil, fmt.Errorf("failed to fetch server actor profile: %w", err)
	}

	var profile struct {
		Inbox       string `json:"inbox"`
		SharedInbox string `json:"sharedInbox"`
		Endpoints   struct {
			SharedInbox string `json:"sharedInbox"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(profileBody, &profile); err != nil {
		return "", nil, fmt.Errorf("failed to parse actor profile: %w", err)
	}

	resolvedSharedInbox := profile.Endpoints.SharedInbox
	if resolvedSharedInbox == "" {
		resolvedSharedInbox = profile.SharedInbox
	}
	if resolvedSharedInbox == "" {
		resolvedSharedInbox = profile.Inbox
	}

	if resolvedSharedInbox == "" {
		return "", nil, fmt.Errorf("no shared inbox or inbox found in server actor profile")
	}

	return resolvedSharedInbox, profileBody, nil
}

// cacheServerInbox persists the discovered shared inbox as quads so future queries are instant
func (s *ActivityService) cacheServerInbox(ctx context.Context, targetDomain, sharedInbox string, profileBody []byte) {
	graphID, _ := s.storage.CreateGraphVersion(ctx, "https://"+targetDomain, "https://"+targetDomain, profileBody)
	if graphID > 0 {
		quads := []model.Quad{
			{
				GraphID:   graphID,
				Subject:   "https://" + targetDomain,
				Predicate: "https://www.w3.org/ns/activitystreams#sharedInbox",
				Object:    sharedInbox,
				ObjType:   model.NamedNode,
			},
		}
		_ = s.storage.SaveQuads(ctx, quads)
	}
}

func (s *ActivityService) signedGet(ctx context.Context, targetURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/activity+json, application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\", application/jrd+json")
	req.Header.Set("User-Agent", "Sprezz-Hex-QuadStore/2.0")

	if privateKeyRSAPEM != "" && keyID != "" {
		cleanHost := req.URL.Host
		if host, _, err := net.SplitHostPort(req.URL.Host); err == nil {
			cleanHost = host
		}
		req.Header.Set("Host", cleanHost)
		dateStr := time.Now().UTC().Format(http.TimeFormat)
		req.Header.Set("Date", dateStr)

		signingString := fmt.Sprintf("(request-target): get %s\nhost: %s\ndate: %s",
			req.URL.RequestURI(), cleanHost, dateStr)

		signature, err := signStringRSA(signingString, privateKeyRSAPEM)
		if err == nil {
			sigHeaderVal := fmt.Sprintf("keyId=\"%s\",algorithm=\"rsa-sha256\",headers=\"(request-target) host date\",signature=\"%s\"",
				keyID, signature)
			req.Header.Set("Signature", sigHeaderVal)
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func signStringRSA(message, privateKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to parse raw identity key block format")
	}

	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		if parsedKey, err8 := x509.ParsePKCS8PrivateKey(block.Bytes); err8 == nil {
			if rsaKey, ok := parsedKey.(*rsa.PrivateKey); ok {
				privKey = rsaKey
			} else {
				return "", fmt.Errorf("key is not RSA private key: %w", err)
			}
		} else {
			return "", err
		}
	}

	msgHash := sha256.Sum256([]byte(message))
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, msgHash[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sigBytes), nil
}
