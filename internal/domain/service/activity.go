package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports"
)

type ActivityService struct {
	storage   ports.StoragePort
	parser    ports.JSONLDParserPort
	forwarder ports.OutboundDispatcher
}

func NewActivityService(storage ports.StoragePort, parser ports.JSONLDParserPort, forwarders ...ports.OutboundDispatcher) *ActivityService {
	service := &ActivityService{
		storage: storage,
		parser:  parser,
	}
	if len(forwarders) > 0 {
		service.forwarder = forwarders[0]
	}
	return service
}

var _ ports.ActivityServicePort = (*ActivityService)(nil)

func (s *ActivityService) ProcessInboundTask(ctx context.Context, task model.InboundTask) error {
	// If the storage instance implements the composite GraphVersionWriter interface,
	// utilize the transaction-wrapped batch writing method.
	if writer, ok := s.storage.(ports.GraphVersionWriter); ok {
		quads, err := s.parser.ToQuads(ctx, 0, task.ObjectIRI, task.Payload)
		if err != nil {
			return fmt.Errorf("Failed to parse activity payload to quads: %w", err)
		}
		if err := writer.SaveGraphVersion(ctx, task.ActivityIRI, task.ObjectIRI, task.Payload, quads); err != nil {
			return fmt.Errorf("Failed to save graph version and quads: %w", err)
		}
		return nil
	}

	// Fallback path utilizing explicit graph versioning combined with the optimized ports layer
	graphID, err := s.storage.CreateGraphVersion(ctx, task.ActivityIRI, task.ObjectIRI, task.Payload)
	if err != nil {
		return fmt.Errorf("Failed to create graph version: %w", err)
	}

	quads, err := s.parser.ToQuads(ctx, graphID, task.ObjectIRI, task.Payload)
	if err != nil {
		return fmt.Errorf("Failed to parse activity payload to quads: %w", err)
	}

	// Updated the fallback loop branch to pipe string quad slices straight through
	// the high-performance SaveQuads adapter method, keeping your storage pipeline fully aligned.
	if err := s.storage.SaveQuads(ctx, quads); err != nil {
		return fmt.Errorf("Failed to save quads: %w", err)
	}

	return nil
}

func (s *ActivityService) DispatchOutboundActivity(ctx context.Context, activityIRI string, actorIRI string, payload []byte) error {
	if s.forwarder == nil {
		return fmt.Errorf("Outbound dispatcher is not configured")
	}
	var envelope struct {
		Inbox string `json:"inbox"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("Decode outbound activity: %w", err)
	}
	if envelope.Inbox == "" {
		return fmt.Errorf("Outbound activity %s has no target inbox", activityIRI)
	}
	privateKey, err := s.storage.GetActorPrivateKey(ctx, actorIRI)
	if err != nil {
		return fmt.Errorf("Load actor private key: %w", err)
	}
	return s.forwarder.ForwardFederatedActivity(ctx, envelope.Inbox, actorIRI+"#main-key", privateKey, payload)
}

func (s *ActivityService) GetFollowersTimeline(ctx context.Context, actorIRI string, limit, offset int) ([]string, error) {
	quads, err := s.storage.StreamQuadsBySubject(ctx, actorIRI)
	if err != nil {
		return nil, fmt.Errorf("Failed to stream quads for actor %s: %w", actorIRI, err)
	}

	followers := make([]string, 0)
	for _, q := range quads {
		// Converted from HasSuffix to strings.Contains to catch any W3C protocol layout
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
		return nil, fmt.Errorf("Failed to stream candidate collection payloads: %w", err)
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
