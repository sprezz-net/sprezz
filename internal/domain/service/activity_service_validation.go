package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sprezz/internal/domain/model"
)

// validateInboundActivity runs strict security, origin, identity, and state checks on inbound activities
func (s *ActivityService) validateInboundActivity(ctx context.Context, activityIRI string, payload []byte) error {
	var activity struct {
		ID         string      `json:"id"`
		Type       string      `json:"type"`
		Actor      interface{} `json:"actor"`
		Object     interface{} `json:"object"`
		Target     interface{} `json:"target"`
		Instrument interface{} `json:"instrument"`
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

	return s.validateCoreInteractions(ctx, actorIRI, activity.Type, activity.Object, activity.Target, activity.Instrument)
}

func (s *ActivityService) validateCoreInteractions(ctx context.Context, actorIRI, actType string, object, target, instrument interface{}) error {
	switch actType {
	case model.ShortCreate:
		return s.validateCreateVerb(ctx, actorIRI, object)
	case model.ShortAccept, model.ShortReject:
		return s.validateAcceptRejectVerb(ctx, actorIRI, actType, object)
	case model.ShortAdd, model.ShortRemove:
		return s.validateAddRemoveVerb(ctx, actorIRI, target, object)
	case model.ShortLike, model.ShortDislike, model.ShortEmojiReact:
		return s.validateLikeDislikeVerb(ctx, actorIRI, actType, object)
	case model.ShortAnnounce:
		return s.validateAnnounceVerb(ctx, object)
	case model.ShortJoin, model.ShortLeave:
		return s.validateJoinLeaveVerb(ctx, object)
	case model.ShortQuestion:
		return s.validateQuestionVerb(ctx, actorIRI, actType, object)
	case model.ShortQuoteRequest, model.TypeQuoteRequest:
		return s.validateQuoteRequestVerb(actorIRI, object, instrument)
	}
	return nil
}

func (s *ActivityService) resolveTenantID(ctx context.Context, activityIRI string) int32 {
	tenantID, _ := ctx.Value(model.TenantIDKey).(int32)
	if tenantID != 0 {
		return tenantID
	}
	if activityIRI != "" {
		if lookupID, err := s.storage.GetTenantIDByActivityIRI(ctx, activityIRI); err == nil && lookupID != 0 {
			return lookupID
		}
	}
	return 1
}

func (s *ActivityService) getOriginalActor(targetMap *ThreadSafePredicateMap) string {
	for pred, objects := range targetMap.m {
		if pred == model.PredicateActor || pred == model.PredicateAttributedTo {
			if len(objects) > 0 {
				return strings.Trim(objects[0], `"'`)
			}
		}
	}
	return ""
}

func (s *ActivityService) validateMutatingVerb(ctx context.Context, activityIRI, actorIRI, actType string, object interface{}) error {
	targetIRI := parseStringOrID(object)
	if targetIRI == "" {
		return fmt.Errorf("missing object IRI for side-effect verb %s", actType)
	}

	actorQuads, err := s.storage.StreamQuadsBySubject(ctx, actorIRI)
	if err != nil {
		return fmt.Errorf("failed to stream quads for actor %s: %w", actorIRI, err)
	}
	actorMap := NewThreadSafePredicateMap(actorQuads)
	if !actorMap.HasKey(model.PredicatePublicKeyPem) {
		return fmt.Errorf("security violation: actor %s does not have an active public key graph entry", actorIRI)
	}

	tenantID := s.resolveTenantID(ctx, activityIRI)
	targetQuads, err := s.storage.GetStatementsBySubjectIsolated(ctx, targetIRI, tenantID)
	if err != nil {
		return fmt.Errorf("failed to stream isolated quads for target IRI %s: %w", targetIRI, err)
	}

	targetMap := NewThreadSafePredicateMap(targetQuads)
	if targetMap.Len() == 0 {
		return ErrDropAction
	}

	originalActor := s.getOriginalActor(targetMap)
	if originalActor != "" && strings.TrimSpace(actorIRI) != strings.TrimSpace(originalActor) {
		return fmt.Errorf("security violation: actor %s is not identical to original target actor %s", actorIRI, originalActor)
	}
	return nil
}

func (s *ActivityService) validateCreateVerb(ctx context.Context, actorIRI string, object interface{}) error {
	objID := parseStringOrID(object)
	if objID != "" {
		actorDomain := extractDomain(actorIRI)
		objectDomain := extractDomain(objID)
		if actorDomain != "" && objectDomain != "" && actorDomain != objectDomain {
			return fmt.Errorf("security violation: actor domain %s does not match object origin domain %s", actorDomain, objectDomain)
		}
	}

	if err := s.validateActorSelfCreation(actorIRI, object); err != nil {
		return err
	}

	return s.validateQuotePostConsent(ctx, actorIRI, object)
}

// validateActorSelfCreation enforces that only the actor themselves can create/represent their own profile.
func (s *ActivityService) validateActorSelfCreation(actorIRI string, object interface{}) error {
	validateMap := func(objMap map[string]interface{}) error {
		objType := SafeExtractString(objMap["type"])
		if objType == "" {
			objType = SafeExtractString(objMap["@type"])
		}
		if objType == "" {
			return nil
		}

		if !model.IsShortActorType(objType) && !model.IsActorType(objType) {
			return nil
		}

		objID := SafeExtractString(objMap["id"])
		if objID == "" {
			objID = SafeExtractString(objMap["@id"])
		}
		if objID == "" {
			return nil
		}

		if actorIRI != objID {
			return fmt.Errorf("security violation: actor %s is not authorized to create actor profile %s", actorIRI, objID)
		}

		return nil
	}

	return ExecuteOnHeterogeneousObjects(object, validateMap)
}

func (s *ActivityService) extractFollowState(targetMap *ThreadSafePredicateMap) (string, string, bool) {
	var originalTarget, originalActor string
	hasState := false
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
	return originalTarget, originalActor, hasState
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

	originalTarget, originalActor, hasState := s.extractFollowState(targetMap)
	if hasState {
		return fmt.Errorf("prior activity %s is not in a pending state", targetIRI)
	}

	if originalTarget != "" && strings.TrimSpace(actorIRI) != strings.TrimSpace(originalTarget) {
		return fmt.Errorf("security violation: actor %s is not authorized to %s follow sent by %s", actorIRI, activityType, originalActor)
	}
	return nil
}

func (s *ActivityService) getCollectionOwner(ctx context.Context, collectionIRI string) (string, error) {
	colQuads, err := s.storage.StreamQuadsBySubject(ctx, collectionIRI)
	if err != nil || len(colQuads) == 0 {
		return "", nil
	}
	colMap := NewThreadSafePredicateMap(colQuads)
	for pred, objects := range colMap.m {
		if pred == model.PredicateActor || pred == model.PredicateAttributedTo {
			if len(objects) > 0 {
				return strings.Trim(objects[0], `"'`), nil
			}
		}
	}
	return "", nil
}

func (s *ActivityService) isCollectionPubliclyAppendable(ctx context.Context, collectionIRI string) bool {
	colQuads, err := s.storage.StreamQuadsBySubject(ctx, collectionIRI)
	if err != nil || len(colQuads) == 0 {
		return false
	}
	colMap := NewThreadSafePredicateMap(colQuads)
	for pred, objects := range colMap.m {
		if pred == model.PredicatePublicAppend {
			for _, obj := range objects {
				cleanVal := strings.ToLower(strings.Trim(obj, `"'`))
				if cleanVal == "true" || cleanVal == "1" {
					return true
				}
			}
		}
	}
	return false
}

func (s *ActivityService) validateAddRemoveVerb(ctx context.Context, actorIRI string, target, object interface{}) error {
	collectionIRI := parseStringOrID(target)
	if collectionIRI == "" {
		collectionIRI = parseStringOrID(object)
	}
	if collectionIRI != "" {
		ownerActor, err := s.getCollectionOwner(ctx, collectionIRI)
		if err == nil && ownerActor != "" && strings.TrimSpace(actorIRI) != strings.TrimSpace(ownerActor) {
			if !s.isCollectionPubliclyAppendable(ctx, collectionIRI) {
				return fmt.Errorf("security violation: actor %s is not authorized to edit collection %s owned by %s", actorIRI, collectionIRI, ownerActor)
			}
		}
	}
	return nil
}

func (s *ActivityService) verifyLikePrivacy(actorIRI, targetIRI string, isPublic bool, originalActor string, recipients []string) error {
	if isPublic || originalActor == "" || strings.TrimSpace(actorIRI) == strings.TrimSpace(originalActor) {
		return nil
	}
	for _, r := range recipients {
		if strings.TrimSpace(actorIRI) == strings.TrimSpace(r) {
			return nil
		}
	}
	return fmt.Errorf("security violation: actor %s does not have privacy clearance to view private object %s", actorIRI, targetIRI)
}

func (s *ActivityService) verifyDuplicateLike(ctx context.Context, actorIRI, targetIRI string) error {
	actorQuads, _ := s.storage.StreamQuadsBySubject(ctx, actorIRI)
	actorMap := NewThreadSafePredicateMap(actorQuads)
	for pred, objects := range actorMap.m {
		if pred == model.PredicateLiked {
			for _, obj := range objects {
				if strings.Trim(obj, `"'`) == targetIRI {
					return fmt.Errorf("idempotency violation: actor %s has already liked object %s", actorIRI, targetIRI)
				}
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

	if err := s.verifyLikePrivacy(actorIRI, targetIRI, isPublic, originalActor, recipients); err != nil {
		return err
	}

	if actType == model.ShortLike {
		if err := s.verifyDuplicateLike(ctx, actorIRI, targetIRI); err != nil {
			return err
		}
	}
	return nil
}

func (s *ActivityService) extractRecipientsFromObjects(objects []string, isPublic *bool) []string {
	var recipients []string
	for _, obj := range objects {
		cleanObject := strings.Trim(obj, `"'`)
		if cleanObject == model.PublicAudience {
			*isPublic = true
		} else {
			recipients = append(recipients, cleanObject)
		}
	}
	return recipients
}

func (s *ActivityService) extractVisibilityAndActor(targetMap *ThreadSafePredicateMap) (bool, string, []string) {
	isPublic := false
	var originalActor string
	var recipients []string
	for pred, objects := range targetMap.m {
		if pred == model.PredicateActor || pred == model.PredicateAttributedTo {
			if len(objects) > 0 {
				originalActor = strings.Trim(objects[0], `"'`)
			}
		}
		if isAddressingPredicate(pred) {
			recipients = append(recipients, s.extractRecipientsFromObjects(objects, &isPublic)...)
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

	targets := append(SafeExtractStringSlice(remoteObj.To), SafeExtractStringSlice(remoteObj.Cc)...)
	targets = append(targets, SafeExtractStringSlice(remoteObj.Audience)...)

	for _, t := range targets {
		if t == model.PublicAudience {
			return true
		}
	}
	return false
}

func (s *ActivityService) isCachedObjectPublic(targetMap *ThreadSafePredicateMap) bool {
	isPublic := false
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

func (s *ActivityService) hasGroupOrCollectionType(targetMap *ThreadSafePredicateMap) bool {
	for pred, objects := range targetMap.m {
		if pred == model.RDFType {
			for _, obj := range objects {
				if model.IsGroupOrCollectionType(obj) {
					return true
				}
			}
		}
	}
	return false
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
	if !s.hasGroupOrCollectionType(targetMap) {
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
	if hasVoted {
		return fmt.Errorf("double-vote violation: actor %s has already voted on Question %s", actorIRI, targetIRI)
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
