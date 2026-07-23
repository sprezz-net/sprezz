package service_test

import (
	"context"
	"errors"
	"testing"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports"
	"sprezz/internal/domain/service"
)

var _ ports.StoragePort = (*MockStorageAdapter)(nil)

type MockStorageAdapter struct {
	OnCreateGraphVersion   func(activityIRI, objectIRI string, payload []byte) (int64, error)
	OnSaveQuads            func(quads []model.Quad) error
	OnStreamQuadsBySubject func(subjectIRI string) ([]model.Quad, error)
}

func (m *MockStorageAdapter) IsDomainBlocked(ctx context.Context, d string) (bool, error) {
	return false, nil
}
func (m *MockStorageAdapter) EnqueueInbound(ctx context.Context, id, a, o, t string, p []byte) error {
	return nil
}
func (m *MockStorageAdapter) ClaimInboundBatch(ctx context.Context, b int) ([]model.InboundTask, error) {
	return nil, nil
}
func (m *MockStorageAdapter) MarkInboundComplete(ctx context.Context, id string) error  { return nil }
func (m *MockStorageAdapter) MarkInboundFailed(ctx context.Context, id, r string) error { return nil }
func (m *MockStorageAdapter) RemoveQuadEdge(ctx context.Context, s, p, o string) error  { return nil }
func (m *MockStorageAdapter) GetLatestPayload(ctx context.Context, o string) ([]byte, error) {
	return nil, nil
}

func (m *MockStorageAdapter) StreamQuadsBySubject(ctx context.Context, s string) ([]model.Quad, error) {
	if m.OnStreamQuadsBySubject != nil {
		return m.OnStreamQuadsBySubject(s)
	}
	return nil, nil
}

func (m *MockStorageAdapter) GetNomadicIdentity(ctx context.Context, guid string) (*model.NomadicIdentity, error) {
	return &model.NomadicIdentity{GUID: guid}, nil
}
func (m *MockStorageAdapter) UpsertNomadicIdentity(ctx context.Context, identity *model.NomadicIdentity) error {
	return nil
}
func (m *MockStorageAdapter) RegisterIdentityClone(ctx context.Context, guid string, hubURL string, isLocal bool) error {
	return nil
}
func (m *MockStorageAdapter) GetActorPrivateKey(ctx context.Context, a string) (string, error) {
	return "", nil
}

func (m *MockStorageAdapter) CreateGraphVersion(ctx context.Context, activityIRI, objectIRI string, rawPayload []byte) (int64, error) {
	if m.OnCreateGraphVersion != nil {
		return m.OnCreateGraphVersion(activityIRI, objectIRI, rawPayload)
	}
	return 1, nil
}

func (m *MockStorageAdapter) SaveQuads(ctx context.Context, quads []model.Quad) error {
	if m.OnSaveQuads != nil {
		return m.OnSaveQuads(quads)
	}
	return nil
}

var _ ports.JSONLDParserPort = (*MockParserAdapter)(nil)

type MockParserAdapter struct {
	OnToQuads func(graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error)
}

func (m *MockParserAdapter) ToQuads(ctx context.Context, graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
	if m.OnToQuads != nil {
		return m.OnToQuads(graphID, mainObjectIRI, rawJSON)
	}
	return []model.Quad{}, nil
}

func TestProcessInboundTask_Success(t *testing.T) {
	ctx := context.Background()
	storageInvoked := false
	parserInvoked := false

	mockStorage := &MockStorageAdapter{
		OnCreateGraphVersion: func(activityIRI, objectIRI string, payload []byte) (int64, error) {
			storageInvoked = true
			return 42, nil
		},
		OnSaveQuads: func(quads []model.Quad) error {
			return nil
		},
	}

	mockParser := &MockParserAdapter{
		OnToQuads: func(graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
			parserInvoked = true
			return []model.Quad{
				{GraphID: graphID, Subject: mainObjectIRI, Predicate: "rdf:type", Object: "as:Note", ObjType: model.NamedNode},
			}, nil
		},
	}

	svc := service.NewActivityService(mockStorage, mockParser)
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/1",
		ObjectIRI:   "https://remote.com/note/1",
		Payload:     []byte(`{}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if !storageInvoked || !parserInvoked {
		t.Error("Pipeline ports execution skipped critical sequences")
	}
}

func TestProcessInboundTask_StorageError(t *testing.T) {
	ctx := context.Background()
	mockStorage := &MockStorageAdapter{
		OnCreateGraphVersion: func(activityIRI, objectIRI string, payload []byte) (int64, error) {
			return 0, errors.New("db error")
		},
	}
	svc := service.NewActivityService(mockStorage, &MockParserAdapter{})
	task := model.InboundTask{ID: "task-1"}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error on storage failure, got nil")
	}
}

func TestProcessInboundTask_ParserError(t *testing.T) {
	ctx := context.Background()
	mockStorage := &MockStorageAdapter{
		OnCreateGraphVersion: func(activityIRI, objectIRI string, payload []byte) (int64, error) {
			return 1, nil
		},
	}
	mockParser := &MockParserAdapter{
		OnToQuads: func(graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
			return nil, errors.New("parse error")
		},
	}
	svc := service.NewActivityService(mockStorage, mockParser)
	task := model.InboundTask{ID: "task-1"}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error on parser failure, got nil")
	}
}

func TestGetFollowersTimeline(t *testing.T) {
	ctx := context.Background()
	actorIRI := "https://sprezz.net/actors/alice"
	followerPredicate := "https://www.w3.org/ns/activitystreams#follower"

	mockStorage := &MockStorageAdapter{
		OnStreamQuadsBySubject: func(subjectIRI string) ([]model.Quad, error) {
			if subjectIRI != actorIRI {
				return nil, nil
			}
			return []model.Quad{
				{Subject: actorIRI, Predicate: followerPredicate, Object: "https://remote.com/users/bob"},
				{Subject: actorIRI, Predicate: followerPredicate, Object: "https://remote.com/users/charlie"},
				{Subject: actorIRI, Predicate: followerPredicate, Object: "https://remote.com/users/dave"},
				{Subject: actorIRI, Predicate: "https://schema.org/name", Object: "Alice"},
			}, nil
		},
	}

	svc := service.NewActivityService(mockStorage, &MockParserAdapter{})

	followers, err := svc.GetFollowersTimeline(ctx, actorIRI, 2, 0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(followers) != 2 {
		t.Fatalf("Expected 2 followers, got %d", len(followers))
	}
	if followers[0] != "https://remote.com/users/bob" || followers[1] != "https://remote.com/users/charlie" {
		t.Errorf("Unexpected followers list: %v", followers)
	}

	// Test pagination offset
	followersOffset, err := svc.GetFollowersTimeline(ctx, actorIRI, 2, 2)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(followersOffset) != 1 || followersOffset[0] != "https://remote.com/users/dave" {
		t.Errorf("Unexpected paginated followers list: %v", followersOffset)
	}
}
