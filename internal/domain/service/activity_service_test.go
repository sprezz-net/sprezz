package service_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/domain/port/portstub"
	"sprezz/internal/domain/service"
)

var _ port.StoragePort = (*MockStorageAdapter)(nil)

type MockStorageAdapter struct {
	portstub.UnimplementedStoragePort // Composite fallback embedded stub (de-bloating layout)
	OnCreateGraphVersion              func(activityIRI, objectIRI string, payload []byte) (int64, error)
	OnSaveQuads                       func(quads []model.Quad) error
	OnStreamQuadsBySubject            func(subjectIRI string) ([]model.Quad, error)
	GetCollectionPayloadsFunc         func(ctx context.Context, a, c string, l, o int) ([][]byte, error)

	OnSaveGraphVersion          func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error
	OnSaveGraphVersionWithMedia func(ctx context.Context, params port.MediaAttachmentParams) error

	// Dual-key orchestration mock hooks
	OnGetActorCredentials   func(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error)
	OnCreateActorCredential func(ctx context.Context, actorIRI string, tenantID int32, username string, rsaPEM, edPEM string) error
	OnArchiveKeyHistory     func(ctx context.Context, actorIRI string, keyType string, publicKeyPEM string, validFrom, validTo time.Time) error
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

func (m *MockStorageAdapter) CreateGraphVersion(ctx context.Context, activityIRI, objectIRI string, rawPayload []byte) (int64, error) {
	if m.OnCreateGraphVersion != nil {
		return m.OnCreateGraphVersion(activityIRI, objectIRI, rawPayload)
	}
	return 1, nil
}

func (m *MockStorageAdapter) SaveGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
	if m.OnSaveGraphVersion != nil {
		return m.OnSaveGraphVersion(ctx, activityIRI, objectIRI, payload, quads)
	}
	return nil
}

func (m *MockStorageAdapter) SaveGraphVersionWithMedia(ctx context.Context, params port.MediaAttachmentParams) error {
	if m.OnSaveGraphVersionWithMedia != nil {
		return m.OnSaveGraphVersionWithMedia(ctx, params)
	}
	return nil
}

func (m *MockStorageAdapter) SaveQuads(ctx context.Context, quads []model.Quad) error {
	if m.OnSaveQuads != nil {
		return m.OnSaveQuads(quads)
	}
	return nil
}

func (m *MockStorageAdapter) GetCollectionPayloads(ctx context.Context, a, c string, l, o int) ([][]byte, error) {
	if m.GetCollectionPayloadsFunc != nil {
		return m.GetCollectionPayloadsFunc(ctx, a, c, l, o)
	}
	return nil, nil
}

func (m *MockStorageAdapter) GetActorCredentials(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error) {
	if m.OnGetActorCredentials != nil {
		return m.OnGetActorCredentials(ctx, tenantID, username)
	}
	return "https://sprezz.net", &model.ActorDualKeys{}, nil
}

func (m *MockStorageAdapter) CreateActorCredential(ctx context.Context, actorIRI string, tenantID int32, username string, rsaPEM, edPEM string) error {
	if m.OnCreateActorCredential != nil {
		return m.OnCreateActorCredential(ctx, actorIRI, tenantID, username, rsaPEM, edPEM)
	}
	return nil
}

func (m *MockStorageAdapter) ArchiveKeyHistory(ctx context.Context, actorIRI string, keyType string, publicKeyPEM string, validFrom, validTo time.Time) error {
	if m.OnArchiveKeyHistory != nil {
		return m.OnArchiveKeyHistory(ctx, actorIRI, keyType, publicKeyPEM, validFrom, validTo)
	}
	return nil
}

var _ port.JSONLDParserPort = (*MockParserAdapter)(nil)

type MockParserAdapter struct {
	portstub.UnimplementedJSONLDParserPort // Embedded shared base stub (de-bloating layout)
	OnToQuads                              func(graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error)
}

func (m *MockParserAdapter) ToQuads(ctx context.Context, graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
	if m.OnToQuads != nil {
		return m.OnToQuads(graphID, mainObjectIRI, rawJSON)
	}
	return []model.Quad{}, nil
}

// MockMediaAdapter implements port.MediaStoragePort for testing.
type MockMediaAdapter struct {
	portstub.UnimplementedMediaStoragePort // Embedded shared base stub (de-bloating layout)
	PutObjectFunc                          func(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error)
	DeleteObjectFunc                       func(ctx context.Context, objectName string) error
}

func (m *MockMediaAdapter) PutObject(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error) {
	if m.PutObjectFunc != nil {
		return m.PutObjectFunc(ctx, objectName, reader, contentType)
	}
	return objectName, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", nil
}

func (m *MockMediaAdapter) DeleteObject(ctx context.Context, objectName string) error {
	if m.DeleteObjectFunc != nil {
		return m.DeleteObjectFunc(ctx, objectName)
	}
	return nil
}

func TestProcessInboundTask_Success(t *testing.T) {
	ctx := context.Background()
	storageInvoked := false
	parserInvoked := false

	mockStorage := &MockStorageAdapter{
		// Update this block to capture the transaction-wrapped execution path
		OnSaveGraphVersion: func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
			storageInvoked = true
			return nil
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

	svc := service.NewActivityService(mockStorage, mockParser, &MockMediaAdapter{})
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
		// Configure the mock error inside the active transactional branch
		OnSaveGraphVersion: func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
			return errors.New("db error")
		},
	}

	svc := service.NewActivityService(mockStorage, &MockParserAdapter{}, &MockMediaAdapter{})
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
	svc := service.NewActivityService(mockStorage, mockParser, &MockMediaAdapter{})
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

	svc := service.NewActivityService(mockStorage, &MockParserAdapter{}, &MockMediaAdapter{})

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

func TestActivityService_GetCollectionTimeline_PrivacyScoping(t *testing.T) {
	ctx := context.Background()
	actorIRI := "https://sprezz.net/alice"
	readerBob := "https://remote.com/bob"

	publicPayload := []byte(`{"id":"https://sprezz.net/alice","type":"Create","to":["https://www.w3.org/ns/activitystreams#Public"]}`)
	privatePayload := []byte(`{"id":"https://sprezz.net/alice","type":"Create","to":["https://remote.com/bob"]}`)
	blockedPayload := []byte(`{"id":"https://sprezz.net/alice","type":"Create","to":["https://remote.com/dave"]}`)

	mockStorage := &MockStorageAdapter{
		GetCollectionPayloadsFunc: func(ctx context.Context, a, c string, l, o int) ([][]byte, error) {
			if a == actorIRI && c == "outbox" {
				return [][]byte{publicPayload, privatePayload, blockedPayload}, nil
			}
			return nil, nil
		},
	}

	mockParser := &MockParserAdapter{
		OnToQuads: func(graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
			if string(rawJSON) == string(publicPayload) {
				return []model.Quad{{GraphID: graphID, Subject: "act/1", Predicate: "activitystreams#to", Object: "https://www.w3.org/ns/activitystreams#Public", ObjType: model.NamedNode}}, nil
			}
			if string(rawJSON) == string(privatePayload) {
				return []model.Quad{{GraphID: graphID, Subject: "act/2", Predicate: "activitystreams#to", Object: readerBob, ObjType: model.NamedNode}}, nil
			}
			return []model.Quad{{GraphID: graphID, Subject: "act/3", Predicate: "activitystreams#to", Object: "https://remote.com", ObjType: model.NamedNode}}, nil
		},
	}

	svc := service.NewActivityService(mockStorage, mockParser, &MockMediaAdapter{})

	// Test Case 1: Bob reads Alice's outbox timeline
	bobResults, err := svc.GetCollectionTimeline(ctx, readerBob, actorIRI, "outbox", 10, 0)
	if err != nil {
		t.Fatalf("unexpected execution exception: %v", err)
	}
	if len(bobResults) != 2 {
		t.Errorf("Expected Bob to see 2 activities (Public + Direct Target), got %d", len(bobResults))
	}

	// Test Case 2: Anonymous external reader requests Alice's outbox timeline
	anonResults, err := svc.GetCollectionTimeline(ctx, "", actorIRI, "outbox", 10, 0)
	if err != nil {
		t.Fatalf("unexpected execution exception: %v", err)
	}
	if len(anonResults) != 1 {
		t.Errorf("Expected Anonymous reader to see only 1 Public activity, got %d", len(anonResults))
	}
}

func TestRotateLocalActorKeys_Success(t *testing.T) {
	ctx := context.Background()
	actorIRI := "https://example.com"

	// Mock existing seed private/public credentials configuration setup
	seedKeys, err := model.MintNewKeyPair()
	if err != nil {
		t.Fatalf("failed setup: %v", err)
	}

	archiveCount := 0
	overwriteInvoked := false

	mockStorage := &MockStorageAdapter{
		OnGetActorCredentials: func(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error) {
			return actorIRI, &model.ActorDualKeys{
				PrivateKeyRSAPEM:     seedKeys.RSAPrivatePEM,
				PrivateKeyEd25519PEM: seedKeys.Ed25519PrivatePEM,
			}, nil
		},
		OnArchiveKeyHistory: func(ctx context.Context, targetIRI string, keyType string, pubKeyPEM string, validFrom, validTo time.Time) error {
			archiveCount++
			return nil
		},
		OnCreateActorCredential: func(ctx context.Context, targetIRI string, tenantID int32, username string, rsaPEM, edPEM string) error {
			overwriteInvoked = true
			if rsaPEM == seedKeys.RSAPrivatePEM {
				t.Error("rotation failed to overwrite legacy private key with fresh material")
			}
			return nil
		},
	}

	svc := service.NewActivityService(mockStorage, &MockParserAdapter{}, &MockMediaAdapter{})

	resIRI, err := svc.RotateLocalActorKeys(ctx, 1, "server")
	if err != nil {
		t.Fatalf("unexpected key rotation failure: %v", err)
	}
	if resIRI != actorIRI {
		t.Errorf("expected target IRI %q, got %q", actorIRI, resIRI)
	}
	if archiveCount != 2 {
		t.Errorf("expected exactly 2 archival history logs (RSA + Ed25519), got %d", archiveCount)
	}
	if !overwriteInvoked {
		t.Error("rotation pipeline skipped storage row write sequence")
	}
}
