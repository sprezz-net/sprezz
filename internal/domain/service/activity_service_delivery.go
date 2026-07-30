package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
)

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

func (s *ActivityService) deliverAndForwardInbound(ctx context.Context, task model.InboundTask) error {
	localRecipients, err := s.deliverToLocalInboxes(ctx, task)
	if err != nil {
		return err
	}
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

func (s *ActivityService) getFormerType(ctx context.Context, targetIRI string) string {
	latest, err := s.storage.GetLatestPayload(ctx, targetIRI)
	if err == nil && len(latest) > 0 {
		var latestMap map[string]interface{}
		if json.Unmarshal(latest, &latestMap) == nil {
			if t, ok := latestMap["type"].(string); ok && t != "" {
				return t
			}
		}
	}
	return model.ShortNote
}

func (s *ActivityService) saveTombstone(ctx context.Context, activityIRI, targetIRI string, tombstonePayload []byte) error {
	if writer, ok := s.storage.(port.GraphVersionWriter); ok {
		tombstoneQuads, err := s.parser.ToQuads(ctx, 0, targetIRI, tombstonePayload)
		if err != nil {
			return fmt.Errorf("failed to parse tombstone payload: %w", err)
		}
		if err := writer.SaveGraphVersion(ctx, activityIRI, targetIRI, tombstonePayload, tombstoneQuads); err != nil {
			return fmt.Errorf("failed to save tombstone graph version: %w", err)
		}
		return nil
	}

	graphID, err := s.storage.CreateGraphVersion(ctx, activityIRI, targetIRI, tombstonePayload)
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
	return nil
}

func (s *ActivityService) deliverDeleteActivity(ctx context.Context, task model.InboundTask) error {
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
			if err := s.storage.RecordActorInboxDelivery(ctx, target, task.ActivityIRI); err != nil {
				return fmt.Errorf("failed to record delete activity delivery for %s: %w", target, err)
			}
		}
	}
	return nil
}

// processDeleteActivity executes the state changes for a deleted object resource.
func (s *ActivityService) processDeleteActivity(ctx context.Context, task model.InboundTask, object interface{}) error {
	targetIRI := parseStringOrID(object)
	if targetIRI == "" {
		return nil
	}
	formerType := s.getFormerType(ctx, targetIRI)

	tombstone := map[string]interface{}{
		"id":         targetIRI,
		"type":       model.ShortTombstone,
		"formerType": formerType,
		"deleted":    time.Now().UTC().Format(time.RFC3339),
	}
	tombstonePayload, _ := json.Marshal(tombstone)

	if err := s.saveTombstone(ctx, task.ActivityIRI, targetIRI, tombstonePayload); err != nil {
		return err
	}

	return s.deliverDeleteActivity(ctx, task)
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
			if err := s.storage.RecordActorInboxDelivery(ctx, target, task.ActivityIRI); err != nil {
				return nil, fmt.Errorf("failed to record actor inbox delivery for %s: %w", target, err)
			}
		}
	}
	return localRecipients, nil
}

func (s *ActivityService) getRemoteForwardingTargets(ctx context.Context, payload []byte) (map[string]struct{}, error) {
	origTargets, err := extractAddressingTargets(payload)
	if err != nil {
		return nil, err
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
	return remoteTargets, nil
}

func (s *ActivityService) getSigningCredentials(ctx context.Context, localRecipient string) (string, string, string, string) {
	serverActorIRI, serverKeys, err := s.getLocalServerCredentials(ctx, localRecipient)
	if err == nil {
		return serverActorIRI, serverActorIRI + model.SuffixMainKey, serverKeys.PrivateKeyRSAPEM, serverKeys.PrivateKeyEd25519PEM
	}
	dualKeys, err := s.storage.GetActorDualKeys(ctx, localRecipient)
	if err == nil {
		return localRecipient, localRecipient + model.SuffixMainKey, dualKeys.PrivateKeyRSAPEM, dualKeys.PrivateKeyEd25519PEM
	}
	return "", "", "", ""
}

// resolveDomainInboxes resolves the inboxes (shared inbox or individual ones) for targets within a given domain.
func (s *ActivityService) resolveDomainInboxes(ctx context.Context, domain string, recipients []string, serverActorIRI string) map[string]struct{} {
	inboxesMap := make(map[string]struct{})
	sharedInboxURL, err := s.resolveServerActorInbox(ctx, domain, serverActorIRI)
	if err == nil && sharedInboxURL != "" {
		inboxesMap[sharedInboxURL] = struct{}{}
		return inboxesMap
	}

	for _, target := range recipients {
		inboxURL, err := s.resolveActorInbox(ctx, target)
		if err == nil && inboxURL != "" {
			inboxesMap[inboxURL] = struct{}{}
		}
	}
	return inboxesMap
}

// performInboxForwarding handles relaying thread replies and shared content to related remote servers.
func (s *ActivityService) performInboxForwarding(ctx context.Context, task model.InboundTask, localRecipients []string) error {
	if len(localRecipients) == 0 || s.forwarder == nil {
		return nil
	}

	remoteTargets, err := s.getRemoteForwardingTargets(ctx, task.Payload)
	if err != nil || len(remoteTargets) == 0 {
		return nil
	}

	domainToRecipients := s.groupRemoteTargetsByDomain(ctx, remoteTargets)
	serverActorIRI, targetKeyID, privateKeyRSAPEM, privateKeyEd25519PEM := s.getSigningCredentials(ctx, localRecipients[0])
	if privateKeyRSAPEM == "" {
		return nil
	}

	for domain, recipients := range domainToRecipients {
		hasRel, _ := s.hasRelationshipWithDomain(ctx, localRecipients, domain)
		if !hasRel {
			continue
		}

		inboxesMap := s.resolveDomainInboxes(ctx, domain, recipients, serverActorIRI)

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

func (s *ActivityService) resolveRecipientInboxes(ctx context.Context, inboxesMap map[string]struct{}, recipients []string) {
	for _, target := range recipients {
		inboxURL, err := s.resolveActorInbox(ctx, target)
		if err == nil && inboxURL != "" {
			inboxesMap[inboxURL] = struct{}{}
		}
	}
}

func (s *ActivityService) resolveOutboundInboxes(ctx context.Context, actorIRI string, targetsMap map[string]struct{}, payload []byte, activityIRI string) (map[string]struct{}, error) {
	inboxesMap := make(map[string]struct{})
	domainToRecipients := s.groupRemoteTargetsByDomain(ctx, targetsMap)

	for domain, recipients := range domainToRecipients {
		sharedInboxURL, err := s.resolveServerActorInbox(ctx, domain, actorIRI)
		if err == nil && sharedInboxURL != "" {
			inboxesMap[sharedInboxURL] = struct{}{}
		} else {
			s.resolveRecipientInboxes(ctx, inboxesMap, recipients)
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

// expandFollowersForTarget checks if a target represents a followers collection, resolves the local followers, and updates targets/targetsMap.
func (s *ActivityService) expandFollowersForTarget(ctx context.Context, target string, targetsMap map[string]struct{}, targets *[]string) {
	if !strings.HasSuffix(target, "/followers") {
		return
	}
	localActorIRI := strings.TrimSuffix(target, "/followers")
	if _, err := s.storage.GetActorDualKeys(ctx, localActorIRI); err != nil {
		return
	}
	followers, err := s.GetFollowersTimeline(ctx, localActorIRI, 1000, 0)
	if err != nil {
		return
	}
	for _, follower := range followers {
		if _, exists := targetsMap[follower]; !exists {
			targetsMap[follower] = struct{}{}
			*targets = append(*targets, follower)
		}
	}
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
		s.expandFollowersForTarget(ctx, target, targetsMap, &targets)
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
	if err := s.storage.SaveQuads(ctx, stateQuads); err != nil {
		return fmt.Errorf("failed to save follow state transition: %w", err)
	}

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
