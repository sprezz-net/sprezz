package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/pkg/httputil"

	"github.com/google/uuid"
)

var ErrDropAction = errors.New("drop action gracefully")

type ActivityService struct {
	storage      port.StoragePort
	mediaStorage port.MediaStoragePort
	parser       port.JSONLDParserPort
	forwarder    port.OutboundDispatcher
	fetcher      port.RemoteFetcher
}

func NewActivityService(storage port.StoragePort, parser port.JSONLDParserPort, media port.MediaStoragePort, fetcher port.RemoteFetcher, forwarders ...port.OutboundDispatcher) *ActivityService {
	service := &ActivityService{
		storage:      storage,
		mediaStorage: media,
		parser:       parser,
		fetcher:      fetcher,
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
		if q.Predicate == model.PredicateSharedInbox {
			sharedInbox = strings.Trim(q.Object, `"'`)
		}
		if q.Predicate == model.PredicateInbox {
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

// parseStringOrID extracts a string IRI from a variety of JSON formats (string, nested map, or list)
func parseStringOrID(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		if id, ok := v["id"].(string); ok {
			return id
		}
	case []interface{}:
		if len(v) > 0 {
			return parseStringOrID(v[0])
		}
	}
	return ""
}

// ThreadSafePredicateMap acts as an O(1) thread-safe query lookup cache for target IRI properties.
type ThreadSafePredicateMap struct {
	mu sync.RWMutex
	m  map[string][]string
}

// NewThreadSafePredicateMap converts a raw slice of quads into a thread-safe predicate map.
func NewThreadSafePredicateMap(quads []model.Quad) *ThreadSafePredicateMap {
	m := make(map[string][]string)
	for _, q := range quads {
		m[q.Predicate] = append(m[q.Predicate], q.Object)
	}
	return &ThreadSafePredicateMap{m: m}
}

// Get retrieves all objects matching a given predicate.
func (t *ThreadSafePredicateMap) Get(predicate string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.m[predicate]
}

// HasKey checks if the given predicate exists.
func (t *ThreadSafePredicateMap) HasKey(predicate string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, ok := t.m[predicate]
	return ok
}

// Len returns the number of distinct predicates cached in the map.
func (t *ThreadSafePredicateMap) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.m)
}

// validateInboundActivity runs strict security, origin, identity, and state checks on inbound activities
func (s *ActivityService) validateInboundActivity(ctx context.Context, activityIRI string, payload []byte) error {
	var activity struct {
		ID     string      `json:"id"`
		Type   string      `json:"type"`
		Actor  interface{} `json:"actor"`
		Object interface{} `json:"object"`
		Target interface{} `json:"target"`
	}
	if err := json.Unmarshal(payload, &activity); err != nil {
		return nil
	}

	actorIRI := parseStringOrID(activity.Actor)
	if actorIRI == "" {
		return nil
	}

	if activity.Type == model.ShortUndo || activity.Type == model.ShortDelete || activity.Type == model.ShortUpdate {
		if err := s.validateMutatingVerb(ctx, activityIRI, actorIRI, activity.Type, activity.Object); err != nil {
			return err
		}
	}

	// C. Core Interactions
	switch activity.Type {
	case model.ShortCreate:
		if err := s.validateCreateVerb(actorIRI, activity.Object); err != nil {
			return err
		}
	case model.ShortAccept, model.ShortReject:
		if err := s.validateAcceptRejectVerb(ctx, actorIRI, activity.Type, activity.Object); err != nil {
			return err
		}
	case model.ShortAdd, model.ShortRemove:
		if err := s.validateAddRemoveVerb(ctx, actorIRI, activity.Target, activity.Object); err != nil {
			return err
		}
	case model.ShortLike, model.ShortDislike:
		if err := s.validateLikeDislikeVerb(ctx, actorIRI, activity.Type, activity.Object); err != nil {
			return err
		}
	case model.ShortAnnounce:
		if err := s.validateAnnounceVerb(ctx, activity.Object); err != nil {
			return err
		}
	case model.ShortJoin, model.ShortLeave:
		if err := s.validateJoinLeaveVerb(ctx, activity.Object); err != nil {
			return err
		}
	case model.ShortQuestion:
		if err := s.validateQuestionVerb(ctx, actorIRI, activity.Type, activity.Object); err != nil {
			return err
		}
	}

	return nil
}

func (s *ActivityService) validateMutatingVerb(ctx context.Context, activityIRI, actorIRI, actType string, object interface{}) error {
	targetIRI := parseStringOrID(object)
	if targetIRI == "" {
		return fmt.Errorf("missing object IRI for side-effect verb %s", actType)
	}

	// 1. Cross-validate against internal RDF security public key graph entries
	actorQuads, err := s.storage.StreamQuadsBySubject(ctx, actorIRI)
	if err != nil {
		return fmt.Errorf("failed to stream quads for actor %s: %w", actorIRI, err)
	}
	actorMap := NewThreadSafePredicateMap(actorQuads)
	if !actorMap.HasKey(model.PredicatePublicKeyPem) {
		return fmt.Errorf("security violation: actor %s does not have an active public key graph entry", actorIRI)
	}

	// Resolve tenant ID
	tenantID, _ := ctx.Value(model.TenantIDKey).(int32)
	if tenantID == 0 {
		if activityIRI != "" {
			var lookupErr error
			tenantID, lookupErr = s.storage.GetTenantIDByActivityIRI(ctx, activityIRI)
			if lookupErr != nil || tenantID == 0 {
				tenantID = 1
			}
		} else {
			tenantID = 1
		}
	}

	targetQuads, err := s.storage.GetStatementsBySubjectIsolated(ctx, targetIRI, tenantID)
	if err != nil {
		return fmt.Errorf("failed to stream isolated quads for target IRI %s: %w", targetIRI, err)
	}

	targetMap := NewThreadSafePredicateMap(targetQuads)

	// Enforce graceful fallback pattern
	if targetMap.Len() == 0 {
		return ErrDropAction
	}

	// Programmatic Identity Constraint Check
	var originalActor string
	targetMap.mu.RLock()
	for pred, objects := range targetMap.m {
		if pred == model.PredicateActor || pred == model.PredicateAttributedTo {
			if len(objects) > 0 {
				originalActor = strings.Trim(objects[0], `"'`)
				break
			}
		}
	}
	targetMap.mu.RUnlock()

	if originalActor != "" && strings.TrimSpace(actorIRI) != strings.TrimSpace(originalActor) {
		return fmt.Errorf("security violation: actor %s is not identical to original target actor %s", actorIRI, originalActor)
	}
	return nil
}

func (s *ActivityService) validateCreateVerb(actorIRI string, object interface{}) error {
	objID := parseStringOrID(object)
	if objID != "" {
		actorDomain := extractDomain(actorIRI)
		objectDomain := extractDomain(objID)
		if actorDomain != "" && objectDomain != "" && actorDomain != objectDomain {
			return fmt.Errorf("security violation: actor domain %s does not match object origin domain %s", actorDomain, objectDomain)
		}
	}
	return nil
}

func (s *ActivityService) validateAcceptRejectVerb(ctx context.Context, actorIRI, activityType string, object interface{}) error {
	targetIRI := parseStringOrID(object)
	if targetIRI == "" {
		return fmt.Errorf("missing object IRI for %s", activityType)
	}

	targetQuads, err := s.storage.StreamQuadsBySubject(ctx, targetIRI)
	if err != nil {
		return fmt.Errorf("prior pending activity %s not found in database", targetIRI)
	}
	targetMap := NewThreadSafePredicateMap(targetQuads)
	if targetMap.Len() == 0 {
		return fmt.Errorf("prior pending activity %s not found in database", targetIRI)
	}

	var originalTarget, originalActor string
	hasState := false
	targetMap.mu.RLock()
	for pred, objects := range targetMap.m {
		if pred == model.PredicateActor || pred == model.PredicateAttributedTo {
			if len(objects) > 0 {
				originalActor = strings.Trim(objects[0], `"'`)
			}
		}
		if pred == model.PredicateObject {
			if len(objects) > 0 {
				originalTarget = strings.Trim(objects[0], `"'`)
			}
		}
		if pred == model.PredicateAccepted || pred == model.PredicateRejected || pred == model.PredicateResult {
			hasState = true
		}
	}
	targetMap.mu.RUnlock()

	if hasState {
		return fmt.Errorf("prior activity %s is not in a pending state", targetIRI)
	}

	if originalTarget != "" && strings.TrimSpace(actorIRI) != strings.TrimSpace(originalTarget) {
		return fmt.Errorf("security violation: actor %s is not authorized to %s follow sent by %s", actorIRI, activityType, originalActor)
	}
	return nil
}

func (s *ActivityService) validateAddRemoveVerb(ctx context.Context, actorIRI string, target, object interface{}) error {
	collectionIRI := parseStringOrID(target)
	if collectionIRI == "" {
		collectionIRI = parseStringOrID(object)
	}
	if collectionIRI != "" {
		colQuads, err := s.storage.StreamQuadsBySubject(ctx, collectionIRI)
		if err == nil && len(colQuads) > 0 {
			colMap := NewThreadSafePredicateMap(colQuads)
			var ownerActor string
			colMap.mu.RLock()
			for pred, objects := range colMap.m {
				if pred == model.PredicateActor || pred == model.PredicateAttributedTo {
					if len(objects) > 0 {
						ownerActor = strings.Trim(objects[0], `"'`)
						break
					}
				}
			}
			colMap.mu.RUnlock()
			if ownerActor != "" && strings.TrimSpace(actorIRI) != strings.TrimSpace(ownerActor) {
				return fmt.Errorf("security violation: actor %s is not authorized to edit collection %s owned by %s", actorIRI, collectionIRI, ownerActor)
			}
		}
	}
	return nil
}

func (s *ActivityService) validateLikeDislikeVerb(ctx context.Context, actorIRI, actType string, object interface{}) error {
	targetIRI := parseStringOrID(object)
	if targetIRI == "" {
		return nil
	}
	targetQuads, err := s.storage.StreamQuadsBySubject(ctx, targetIRI)
	if err != nil || len(targetQuads) == 0 {
		return fmt.Errorf("target object %s not found locally", targetIRI)
	}

	targetMap := NewThreadSafePredicateMap(targetQuads)
	isPublic, originalActor, recipients := s.extractVisibilityAndActor(targetMap)

	if !isPublic && originalActor != "" && strings.TrimSpace(actorIRI) != strings.TrimSpace(originalActor) {
		allowed := false
		for _, r := range recipients {
			if strings.TrimSpace(actorIRI) == strings.TrimSpace(r) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("security violation: actor %s does not have privacy clearance to view private object %s", actorIRI, targetIRI)
		}
	}

	if actType == model.ShortLike {
		actorQuads, _ := s.storage.StreamQuadsBySubject(ctx, actorIRI)
		actorMap := NewThreadSafePredicateMap(actorQuads)
		actorMap.mu.RLock()
		alreadyLiked := false
		for pred, objects := range actorMap.m {
			if pred == model.PredicateLiked {
				for _, obj := range objects {
					if strings.Trim(obj, `"'`) == targetIRI {
						alreadyLiked = true
						break
					}
				}
			}
			if alreadyLiked {
				break
			}
		}
		actorMap.mu.RUnlock()
		if alreadyLiked {
			return fmt.Errorf("idempotency violation: actor %s has already liked object %s", actorIRI, targetIRI)
		}
	}
	return nil
}

func (s *ActivityService) extractVisibilityAndActor(targetMap *ThreadSafePredicateMap) (bool, string, []string) {
	isPublic := false
	var originalActor string
	var recipients []string
	targetMap.mu.RLock()
	defer targetMap.mu.RUnlock()
	for pred, objects := range targetMap.m {
		if pred == model.PredicateActor || pred == model.PredicateAttributedTo {
			if len(objects) > 0 {
				originalActor = strings.Trim(objects[0], `"'`)
			}
		}
		if isAddressingPredicate(pred) {
			for _, obj := range objects {
				cleanObject := strings.Trim(obj, `"'`)
				if cleanObject == model.PublicAudience {
					isPublic = true
				} else {
					recipients = append(recipients, cleanObject)
				}
			}
		}
	}
	return isPublic, originalActor, recipients
}

func (s *ActivityService) validateAnnounceVerb(ctx context.Context, object interface{}) error {
	targetIRI := parseStringOrID(object)
	if targetIRI == "" {
		return nil
	}
	targetQuads, err := s.storage.StreamQuadsBySubject(ctx, targetIRI)
	if err != nil || len(targetQuads) == 0 {
		fetchedBody, jitErr := s.fetcher.FetchSigned(ctx, targetIRI, "", "", "")
		if jitErr != nil {
			return fmt.Errorf("privacy guard rejection: remote object %s cannot be verified (safe-rejection posture)", targetIRI)
		}
		if !s.isRemoteObjectPublic(fetchedBody) {
			return fmt.Errorf("privacy guard: cannot announce private/limited remote object %s", targetIRI)
		}
	} else {
		targetMap := NewThreadSafePredicateMap(targetQuads)
		if !s.isCachedObjectPublic(targetMap) {
			return fmt.Errorf("privacy guard: cannot announce private/limited object %s", targetIRI)
		}
	}
	return nil
}

func (s *ActivityService) isRemoteObjectPublic(fetchedBody []byte) bool {
	var remoteObj struct {
		To       interface{} `json:"to"`
		Cc       interface{} `json:"cc"`
		Audience interface{} `json:"audience"`
	}
	if json.Unmarshal(fetchedBody, &remoteObj) != nil {
		return false
	}
	isPublic := false
	checkTarget := func(val interface{}) {
		switch v := val.(type) {
		case string:
			if strings.Contains(v, "Public") {
				isPublic = true
			}
		case []interface{}:
			for _, item := range v {
				if str, ok := item.(string); ok && strings.Contains(str, "Public") {
					isPublic = true
				}
			}
		}
	}
	checkTarget(remoteObj.To)
	checkTarget(remoteObj.Cc)
	checkTarget(remoteObj.Audience)
	return isPublic
}

func (s *ActivityService) isCachedObjectPublic(targetMap *ThreadSafePredicateMap) bool {
	isPublic := false
	targetMap.mu.RLock()
	defer targetMap.mu.RUnlock()
	for pred, objects := range targetMap.m {
		if isAddressingPredicate(pred) {
			for _, obj := range objects {
				if strings.Trim(obj, `"'`) == model.PublicAudience {
					isPublic = true
					break
				}
			}
		}
		if isPublic {
			break
		}
	}
	return isPublic
}

func (s *ActivityService) validateJoinLeaveVerb(ctx context.Context, object interface{}) error {
	targetIRI := parseStringOrID(object)
	if targetIRI == "" {
		return nil
	}
	targetQuads, err := s.storage.StreamQuadsBySubject(ctx, targetIRI)
	if err != nil || len(targetQuads) == 0 {
		return fmt.Errorf("target group or collection %s not found", targetIRI)
	}
	targetMap := NewThreadSafePredicateMap(targetQuads)
	isGroupOrCollection := false
	targetMap.mu.RLock()
	for pred, objects := range targetMap.m {
		if pred == model.RDFType {
			for _, obj := range objects {
				if model.IsGroupOrCollection(obj) {
					isGroupOrCollection = true
					break
				}
			}
		}
		if isGroupOrCollection {
			break
		}
	}
	targetMap.mu.RUnlock()
	if !isGroupOrCollection {
		return fmt.Errorf("target scoping violation: %s is not a Group or Collection", targetIRI)
	}
	return nil
}

func (s *ActivityService) validateQuestionVerb(ctx context.Context, actorIRI, actType string, object interface{}) error {
	targetIRI := parseStringOrID(object)
	if targetIRI == "" {
		return nil
	}
	targetQuads, err := s.storage.StreamQuadsBySubject(ctx, targetIRI)
	if err != nil || len(targetQuads) == 0 {
		return nil
	}
	targetMap := NewThreadSafePredicateMap(targetQuads)

	if err := s.checkQuestionExpiration(targetMap, targetIRI); err != nil {
		return err
	}

	if actType != model.ShortUpdate {
		if err := s.checkDoubleVote(targetMap, actorIRI, targetIRI); err != nil {
			return err
		}
	}
	return nil
}

func (s *ActivityService) checkQuestionExpiration(targetMap *ThreadSafePredicateMap, targetIRI string) error {
	targetMap.mu.RLock()
	defer targetMap.mu.RUnlock()
	for pred, objects := range targetMap.m {
		if pred == model.PredicateEndTime {
			for _, obj := range objects {
				endTimeStr := strings.Trim(obj, `"'`)
				if endTime, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
					if time.Now().UTC().After(endTime) {
						return fmt.Errorf("vote rejected: poll Question %s has already expired", targetIRI)
					}
				}
			}
		}
	}
	return nil
}

func (s *ActivityService) checkDoubleVote(targetMap *ThreadSafePredicateMap, actorIRI, targetIRI string) error {
	hasVoted := false
	targetMap.mu.RLock()
	for pred, objects := range targetMap.m {
		if pred == model.PredicateVoted {
			for _, obj := range objects {
				if strings.Trim(obj, `"'`) == actorIRI {
					hasVoted = true
					break
				}
			}
		}
		if hasVoted {
			break
		}
	}
	targetMap.mu.RUnlock()
	if hasVoted {
		return fmt.Errorf("double-vote violation: actor %s has already voted on Question %s", actorIRI, targetIRI)
	}
	return nil
}

// ProcessInboundTask handles incoming standard (non-media) ActivityPub payloads
func (s *ActivityService) ProcessInboundTask(ctx context.Context, task model.InboundTask) error {
	var activity struct {
		Type   string      `json:"type"`
		Actor  interface{} `json:"actor"`
		Object interface{} `json:"object"`
	}
	_ = json.Unmarshal(task.Payload, &activity)

	// A. Check for idempotent delete redundant actions
	isIdempotent, err := s.handleIdempotentDelete(ctx, activity.Type, activity.Object)
	if err != nil {
		return err
	}
	if isIdempotent {
		return nil
	}

	// B. Execute programmatic identity constraint checks on side-effect mutations before any updates
	if err := s.validateInboundActivity(ctx, task.ActivityIRI, task.Payload); err != nil {
		if errors.Is(err, ErrDropAction) {
			return nil // graceful fallback / nil code exit
		}
		return fmt.Errorf("side-effect mutation rejected: %w", err)
	}

	// C. Process actual delete side-effect and tombstone creation/saving
	if activity.Type == model.ShortDelete {
		return s.processDeleteActivity(ctx, task, activity.Object)
	}

	// D. Parse activity payload into RDF quads and version them
	if _, err := s.saveInboundActivityQuads(ctx, task); err != nil {
		return err
	}

	// E. Perform spec-compliant direct and shared local inbox deliveries
	localRecipients, err := s.deliverToLocalInboxes(ctx, task)
	if err != nil {
		return err
	}

	// F. Perform Section 7.1.2 inbox forwarding on behalf of the original author
	return s.performInboxForwarding(ctx, task, localRecipients)
}

// handleIdempotentDelete detects if a delete action is a duplicate attempt on a tombstone.
func (s *ActivityService) handleIdempotentDelete(ctx context.Context, actType string, object interface{}) (bool, error) {
	if actType != model.ShortDelete {
		return false, nil
	}
	targetIRI := parseStringOrID(object)
	if targetIRI == "" {
		return false, nil
	}
	latest, err := s.storage.GetLatestPayload(ctx, targetIRI)
	if err == nil && len(latest) > 0 {
		var latestMap map[string]interface{}
		if json.Unmarshal(latest, &latestMap) == nil {
			if latestMap["type"] == model.ShortTombstone {
				return true, nil // successful idempotent no-op!
			}
		}
	}
	return false, nil
}

// processDeleteActivity executes the state changes for a deleted object resource.
func (s *ActivityService) processDeleteActivity(ctx context.Context, task model.InboundTask, object interface{}) error {
	targetIRI := parseStringOrID(object)
	if targetIRI == "" {
		return nil
	}
	latest, err := s.storage.GetLatestPayload(ctx, targetIRI)
	formerType := model.ShortNote
	if err == nil && len(latest) > 0 {
		var latestMap map[string]interface{}
		if json.Unmarshal(latest, &latestMap) == nil {
			if t, ok := latestMap["type"].(string); ok && t != "" {
				formerType = t
			}
		}
	}

	tombstone := map[string]interface{}{
		"id":         targetIRI,
		"type":       model.ShortTombstone,
		"formerType": formerType,
		"deleted":    time.Now().UTC().Format(time.RFC3339),
	}
	tombstonePayload, _ := json.Marshal(tombstone)

	if writer, ok := s.storage.(port.GraphVersionWriter); ok {
		tombstoneQuads, err := s.parser.ToQuads(ctx, 0, targetIRI, tombstonePayload)
		if err != nil {
			return fmt.Errorf("failed to parse tombstone payload: %w", err)
		}
		if err := writer.SaveGraphVersion(ctx, task.ActivityIRI, targetIRI, tombstonePayload, tombstoneQuads); err != nil {
			return fmt.Errorf("failed to save tombstone graph version: %w", err)
		}
	} else {
		graphID, err := s.storage.CreateGraphVersion(ctx, task.ActivityIRI, targetIRI, tombstonePayload)
		if err != nil {
			return fmt.Errorf("failed to create tombstone graph version: %w", err)
		}
		tombstoneQuads, err := s.parser.ToQuads(ctx, graphID, targetIRI, tombstonePayload)
		if err != nil {
			return fmt.Errorf("failed to parse tombstone payload: %w", err)
		}
		if err := s.storage.SaveQuads(ctx, tombstoneQuads); err != nil {
			return fmt.Errorf("failed to save tombstone quads: %w", err)
		}
	}

	// Perform standard inbox delivery mapping for the Delete activity
	targetsMap, err := extractAddressingTargets(task.Payload)
	if err != nil {
		return err
	}
	if task.ObjectIRI != "" {
		targetsMap[task.ObjectIRI] = struct{}{}
	}
	s.expandFollowers(ctx, targetsMap)
	for target := range targetsMap {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		_, err := s.storage.GetActorDualKeys(ctx, target)
		if err == nil {
			_ = s.storage.RecordActorInboxDelivery(ctx, target, task.ActivityIRI)
		}
	}
	return nil
}

// saveInboundActivityQuads stores standard activity payload versions and parses RDF quads.
func (s *ActivityService) saveInboundActivityQuads(ctx context.Context, task model.InboundTask) ([]model.Quad, error) {
	if writer, ok := s.storage.(port.GraphVersionWriter); ok {
		quads, err := s.parser.ToQuads(ctx, 0, task.ObjectIRI, task.Payload)
		if err != nil {
			return nil, fmt.Errorf("failed to parse activity payload to quads: %w", err)
		}
		if err := writer.SaveGraphVersion(ctx, task.ActivityIRI, task.ObjectIRI, task.Payload, quads); err != nil {
			return nil, fmt.Errorf("failed to save graph version and quads: %w", err)
		}
		return quads, nil
	}

	// Fallback path utilizing explicit graph versioning combined with the optimized ports layer
	graphID, err := s.storage.CreateGraphVersion(ctx, task.ActivityIRI, task.ObjectIRI, task.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create graph version: %w", err)
	}

	quads, err := s.parser.ToQuads(ctx, graphID, task.ObjectIRI, task.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse activity payload to quads: %w", err)
	}

	if err := s.storage.SaveQuads(ctx, quads); err != nil {
		return nil, fmt.Errorf("failed to save quads: %w", err)
	}
	return quads, nil
}

// deliverToLocalInboxes delivers the activity to matching local actor inboxes.
func (s *ActivityService) deliverToLocalInboxes(ctx context.Context, task model.InboundTask) ([]string, error) {
	targetsMap, err := extractAddressingTargets(task.Payload)
	if err != nil {
		return nil, err
	}

	if task.ObjectIRI != "" {
		targetsMap[task.ObjectIRI] = struct{}{}
	}

	s.expandFollowers(ctx, targetsMap)

	var localRecipients []string
	for target := range targetsMap {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		_, err := s.storage.GetActorDualKeys(ctx, target)
		if err == nil {
			localRecipients = append(localRecipients, target)
			_ = s.storage.RecordActorInboxDelivery(ctx, target, task.ActivityIRI)
		}
	}
	return localRecipients, nil
}

// performInboxForwarding handles relaying thread replies and shared content to related remote servers.
func (s *ActivityService) performInboxForwarding(ctx context.Context, task model.InboundTask, localRecipients []string) error {
	if len(localRecipients) == 0 || s.forwarder == nil {
		return nil
	}

	origTargets, err := extractAddressingTargets(task.Payload)
	if err != nil {
		return nil
	}

	remoteTargets := make(map[string]struct{})
	for target := range origTargets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, err := s.storage.GetActorDualKeys(ctx, target); err == nil {
			continue
		}
		remoteTargets[target] = struct{}{}
	}

	if len(remoteTargets) == 0 {
		return nil
	}

	domainToRecipients := s.groupRemoteTargetsByDomain(ctx, remoteTargets)

	// Determine signing credentials
	serverActorIRI, serverKeys, err := s.getLocalServerCredentials(ctx, localRecipients[0])
	var targetKeyID string
	var privateKeyRSAPEM, privateKeyEd25519PEM string
	if err == nil {
		targetKeyID = serverActorIRI + model.SuffixMainKey
		privateKeyRSAPEM = serverKeys.PrivateKeyRSAPEM
		privateKeyEd25519PEM = serverKeys.PrivateKeyEd25519PEM
	} else {
		serverActorIRI = localRecipients[0]
		targetKeyID = serverActorIRI + model.SuffixMainKey
		dualKeys, err := s.storage.GetActorDualKeys(ctx, serverActorIRI)
		if err == nil {
			privateKeyRSAPEM = dualKeys.PrivateKeyRSAPEM
			privateKeyEd25519PEM = dualKeys.PrivateKeyEd25519PEM
		}
	}

	if privateKeyRSAPEM == "" {
		return nil
	}

	for domain, recipients := range domainToRecipients {
		hasRel, _ := s.hasRelationshipWithDomain(ctx, localRecipients, domain)
		if !hasRel {
			continue
		}

		inboxesMap := make(map[string]struct{})
		sharedInboxURL, err := s.resolveServerActorInbox(ctx, domain, serverActorIRI)
		if err == nil && sharedInboxURL != "" {
			inboxesMap[sharedInboxURL] = struct{}{}
		} else {
			for _, target := range recipients {
				inboxURL, err := s.resolveActorInbox(ctx, target)
				if err == nil && inboxURL != "" {
					inboxesMap[inboxURL] = struct{}{}
				}
			}
		}

		for inbox := range inboxesMap {
			_ = s.forwarder.ForwardFederatedActivity(
				ctx,
				inbox,
				targetKeyID,
				privateKeyRSAPEM,
				privateKeyEd25519PEM,
				task.Payload,
			)
		}
	}

	return nil
}

func (s *ActivityService) checkMediaQuota(ctx context.Context, tenantIDStr string, size int64) (int32, error) {
	var resolvedTenantID int32 = 1
	if tenantIDStr != "" {
		if val, err := strconv.ParseInt(tenantIDStr, 10, 32); err == nil {
			resolvedTenantID = int32(val)
		}
	}

	hasQuota, err := s.storage.VerifyIncomingQuota(ctx, resolvedTenantID, size)
	if err != nil {
		return 0, fmt.Errorf("quota audit system interception error: %w", err)
	}
	if !hasQuota {
		return 0, fmt.Errorf("media workflow aborted: storage authorization ceiling threshold exceeded")
	}
	return resolvedTenantID, nil
}

// ProcessInboundMediaTask pipelines a media stream to MinIO and links it transactionally to the graph metadata.
func (s *ActivityService) ProcessInboundMediaTask(ctx context.Context, mediaCtx port.InboundMediaContext, task model.InboundTask) error {
	if s.mediaStorage == nil {
		return fmt.Errorf("media storage engine driver is not configured")
	}

	_, err := s.checkMediaQuota(ctx, mediaCtx.TenantID, mediaCtx.Size)
	if err != nil {
		return err
	}

	stableKey, sha256Hex, err := s.mediaStorage.PutObject(ctx, mediaCtx.ObjectName, mediaCtx.MediaStream, mediaCtx.ContentType)
	if err != nil {
		return fmt.Errorf("media workflow aborted due to storage upload failure: %w", err)
	}

	quads, err := s.parser.ToQuads(ctx, 0, task.ObjectIRI, task.Payload)
	if err != nil {
		_ = s.mediaStorage.DeleteObject(ctx, stableKey)
		return fmt.Errorf("failed to parse activity payload to quads during media task: %w", err)
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

func (s *ActivityService) resolveOutboundInboxes(ctx context.Context, actorIRI string, targetsMap map[string]struct{}, payload []byte, activityIRI string) (map[string]struct{}, error) {
	inboxesMap := make(map[string]struct{})
	domainToRecipients := s.groupRemoteTargetsByDomain(ctx, targetsMap)

	for domain, recipients := range domainToRecipients {
		sharedInboxURL, err := s.resolveServerActorInbox(ctx, domain, actorIRI)
		if err == nil && sharedInboxURL != "" {
			inboxesMap[sharedInboxURL] = struct{}{}
		} else {
			for _, target := range recipients {
				inboxURL, err := s.resolveActorInbox(ctx, target)
				if err == nil && inboxURL != "" {
					inboxesMap[inboxURL] = struct{}{}
				}
			}
		}
	}

	if len(inboxesMap) == 0 {
		var envelope struct {
			Inbox string `json:"inbox"`
		}
		_ = json.Unmarshal(payload, &envelope)
		if envelope.Inbox == "" {
			return nil, fmt.Errorf("outbound activity %s has no target inbox", activityIRI)
		}
		inboxesMap[envelope.Inbox] = struct{}{}
	}
	return inboxesMap, nil
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

// isAddressingPredicate isolates the specific W3C target addressing field checks.
func isAddressingPredicate(predicate string) bool {
	return predicate == model.PredicateTo ||
		predicate == model.PredicateCc ||
		predicate == model.PredicateBto ||
		predicate == model.PredicateBcc ||
		predicate == model.PredicateAudience
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

	targetKeyID := serverActorIRI + model.SuffixMainKey

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
	inboxURL, err := s.resolveActorInbox(ctx, httputil.HTTPSPrefix+targetDomain)
	if err == nil && inboxURL != "" {
		return inboxURL
	}
	inboxURL, err = s.resolveActorInbox(ctx, httputil.HTTPPrefix+targetDomain)
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

	tenantID, err := s.storage.GetTenantIDByDomain(ctx, senderHost)
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
	wfBody, err := s.fetcher.FetchSigned(ctx, webfingerURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM)
	if err != nil {
		// Fallback to http if https fails
		webfingerURL = fmt.Sprintf("http://%s/.well-known/webfinger?resource=http://%s", targetDomain, targetDomain)
		wfBody, err = s.fetcher.FetchSigned(ctx, webfingerURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM)
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
	profileBody, err := s.fetcher.FetchSigned(ctx, actorProfileURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM)
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
	iri := httputil.HTTPSPrefix + targetDomain
	graphID, _ := s.storage.CreateGraphVersion(ctx, iri, iri, profileBody)
	if graphID > 0 {
		quads := []model.Quad{
			{
				GraphID:   graphID,
				Subject:   iri,
				Predicate: "https://www.w3.org/ns/activitystreams#sharedInbox",
				Object:    sharedInbox,
				ObjType:   model.NamedNode,
			},
		}
		_ = s.storage.SaveQuads(ctx, quads)
	}
}

func (s *ActivityService) anyRecipientFollowsDomain(ctx context.Context, localRecipients []string, targetDomain string) bool {
	for _, localActor := range localRecipients {
		followers, err := s.GetFollowersTimeline(ctx, localActor, 1000, 0)
		if err != nil {
			continue
		}
		for _, follower := range followers {
			if extractDomain(follower) == targetDomain {
				return true
			}
		}
	}
	return false
}

// hasRelationshipWithDomain checks if the local server/actor has a federated relationship with the given remote domain.
func (s *ActivityService) hasRelationshipWithDomain(ctx context.Context, localRecipients []string, targetDomain string) (bool, error) {
	blocked, err := s.storage.IsDomainBlocked(ctx, targetDomain)
	if err != nil || blocked {
		return false, nil
	}

	// 1. If we have a cached server inbox for this domain, we have a relationship
	if s.resolveCachedServerInbox(ctx, targetDomain) != "" {
		return true, nil
	}

	// 2. Check if any local recipient has follower relationships on that domain
	return s.anyRecipientFollowsDomain(ctx, localRecipients, targetDomain), nil
}

func (s *ActivityService) parseFollowActivityQuads(quads []model.Quad) (string, string, error) {
	var followerIRI, followedIRI string
	for _, q := range quads {
		pred := strings.ToLower(q.Predicate)
		if strings.Contains(pred, "actor") || strings.Contains(pred, "attributedto") {
			followerIRI = strings.Trim(q.Object, `"'`)
		}
		if strings.Contains(pred, "object") {
			followedIRI = strings.Trim(q.Object, `"'`)
		}
	}
	if followerIRI == "" || followedIRI == "" {
		return "", "", fmt.Errorf("invalid follow activity structure")
	}
	return followerIRI, followedIRI, nil
}

func (s *ActivityService) getOriginalFollow(ctx context.Context, followActivityIRI string) interface{} {
	followPayload, _ := s.storage.GetLatestPayload(ctx, followActivityIRI)
	if len(followPayload) > 0 {
		var parsed map[string]interface{}
		if json.Unmarshal(followPayload, &parsed) == nil {
			return parsed
		}
	}
	return followActivityIRI
}

func (s *ActivityService) saveFollowStateTransition(ctx context.Context, followActivityIRI, followedActorIRI, followerIRI string, accept bool) error {
	statePredicate := model.PredicateRejected
	if accept {
		statePredicate = model.PredicateAccepted
	}

	stateQuads := []model.Quad{
		{
			Subject:   followActivityIRI,
			Predicate: statePredicate,
			Object:    "true",
			ObjType:   model.Literal,
		},
	}
	_ = s.storage.SaveQuads(ctx, stateQuads)

	if accept {
		followerQuads := []model.Quad{
			{
				Subject:   followedActorIRI,
				Predicate: model.PredicateFollower,
				Object:    followerIRI,
				ObjType:   model.NamedNode,
			},
		}
		if err := s.storage.SaveQuads(ctx, followerQuads); err != nil {
			return fmt.Errorf("failed to save follower relationship quads: %w", err)
		}
	}
	return nil
}

func (s *ActivityService) handleFollowResponse(ctx context.Context, followedActorIRI, followActivityIRI string, accept bool) error {
	quads, err := s.storage.StreamQuadsBySubject(ctx, followActivityIRI)
	if err != nil {
		return err
	}
	if len(quads) == 0 {
		return fmt.Errorf("prior activity %s not found in database", followActivityIRI)
	}

	followerIRI, followedIRI, err := s.parseFollowActivityQuads(quads)
	if err != nil {
		return err
	}

	if followedIRI != followedActorIRI {
		return fmt.Errorf("unauthorized: followed actor mismatch")
	}

	activityType := model.ShortReject
	if accept {
		activityType = model.ShortAccept
	}

	originalFollow := s.getOriginalFollow(ctx, followActivityIRI)

	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	activityIRI := fmt.Sprintf("%s/activities/%s-%s", followedActorIRI, strings.ToLower(activityType), id.String())

	responseActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       activityIRI,
		"type":     activityType,
		"actor":    followedActorIRI,
		"object":   originalFollow,
		"to":       []string{followerIRI},
	}
	payload, err := json.Marshal(responseActivity)
	if err != nil {
		return err
	}

	err = s.DispatchOutboundActivity(ctx, activityIRI, followedActorIRI, payload)
	if err != nil {
		return fmt.Errorf("dispatch follow response activity: %w", err)
	}

	return s.saveFollowStateTransition(ctx, followActivityIRI, followedActorIRI, followerIRI, accept)
}

func (s *ActivityService) AcceptFollow(ctx context.Context, followedActorIRI, followActivityIRI string) error {
	return s.handleFollowResponse(ctx, followedActorIRI, followActivityIRI, true)
}

func (s *ActivityService) RejectFollow(ctx context.Context, followedActorIRI, followActivityIRI string) error {
	return s.handleFollowResponse(ctx, followedActorIRI, followActivityIRI, false)
}
