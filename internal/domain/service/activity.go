package service

import (
	"context"
	"encoding/json"
	"fmt"

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
	if writer, ok := s.storage.(ports.GraphVersionWriter); ok {
		quads, err := s.parser.ToQuads(ctx, 0, task.ObjectIRI, task.Payload)
		if err != nil {
			return fmt.Errorf("failed to parse activity payload to quads: %w", err)
		}
		if err := writer.SaveGraphVersion(ctx, task.ActivityIRI, task.ObjectIRI, task.Payload, quads); err != nil {
			return fmt.Errorf("failed to save graph version and quads: %w", err)
		}
		return nil
	}

	graphID, err := s.storage.CreateGraphVersion(ctx, task.ActivityIRI, task.ObjectIRI, task.Payload)
	if err != nil {
		return fmt.Errorf("failed to create graph version: %w", err)
	}

	quads, err := s.parser.ToQuads(ctx, graphID, task.ObjectIRI, task.Payload)
	if err != nil {
		return fmt.Errorf("failed to parse activity payload to quads: %w", err)
	}

	if err := s.storage.SaveQuads(ctx, quads); err != nil {
		return fmt.Errorf("failed to save quads: %w", err)
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
	privateKey, err := s.storage.GetActorPrivateKey(ctx, actorIRI)
	if err != nil {
		return fmt.Errorf("load actor private key: %w", err)
	}
	return s.forwarder.ForwardFederatedActivity(ctx, envelope.Inbox, actorIRI+"#main-key", privateKey, payload)
}

func (s *ActivityService) GetFollowersTimeline(ctx context.Context, actorIRI string, limit, offset int) ([]string, error) {
	quads, err := s.storage.StreamQuadsBySubject(ctx, actorIRI)
	if err != nil {
		return nil, fmt.Errorf("failed to stream quads for actor %s: %w", actorIRI, err)
	}

	followers := make([]string, 0)
	followerPredicate := "https://www.w3.org/ns/activitystreams#follower"
	for _, q := range quads {
		if q.Predicate == followerPredicate {
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
