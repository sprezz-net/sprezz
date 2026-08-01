package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/domain/service"
	"sprezz/internal/pkg/cryptoutil"

	"github.com/gojuno/minimock/v3"
)

func TestProcessInboundTask_Success(t *testing.T) {
	mc := minimock.NewController(t)

	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.SaveGraphVersionMock.Set(func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
		return nil
	})
	mockStorage.GetActorDualKeysMock.Return(nil, errors.New("not local"))

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
		return []model.Quad{
			{GraphID: graphID, Subject: mainObjectIRI, Predicate: "rdf:type", Object: "as:Note", ObjType: model.NamedNode},
		}, nil
	})

	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})
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
}

func TestProcessInboundTask_PayloadSizeLimit(t *testing.T) {
	mc := minimock.NewController(t)

	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{MaxActivitySizeBytes: 10}) // Set a very tiny limit of 10 bytes

	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/1",
		ObjectIRI:   "https://remote.com/note/1",
		Payload:     []byte(`{"this_is_too_long": true}`), // Greater than 10 bytes
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error due to payload size limit, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum limit") {
		t.Fatalf("Expected error message to mention size limit, got: %v", err)
	}
}

func TestProcessInboundTask_InboxDeliveryError(t *testing.T) {
	mc := minimock.NewController(t)

	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.SaveGraphVersionMock.Set(func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
		return nil
	})

	mockStorage.GetActorDualKeysMock.Set(func(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
		if actorIRI == "https://local-domain.com/actor/alice" {
			return &model.ActorDualKeys{}, nil
		}
		return nil, errors.New("not local actor")
	})

	// Setup RecordActorInboxDelivery to return a database write error
	mockStorage.RecordActorInboxDeliveryMock.Return(errors.New("postgres write error"))

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Return([]model.Quad{}, nil)

	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/1",
		ObjectIRI:   "https://remote.com/note/1",
		Payload:     []byte(`{"to":["https://local-domain.com/actor/alice"]}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error on inbox delivery record failure, got nil")
	}
	if !strings.Contains(err.Error(), "failed to record actor inbox delivery") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestProcessInboundTask_SharedInboxFanOut(t *testing.T) {
	mc := minimock.NewController(t)

	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.SaveGraphVersionMock.Set(func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
		return nil
	})

	// Setup GetActorDualKeys expectations for resolving local actors
	mockStorage.GetActorDualKeysMock.Set(func(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
		if actorIRI == "https://local-domain.com/actor/alice" {
			return &model.ActorDualKeys{}, nil
		}
		return nil, errors.New("not local actor")
	})

	// Setup RecordActorInboxDelivery expectations
	deliveryRecorded := false
	mockStorage.RecordActorInboxDeliveryMock.Set(func(ctx context.Context, actorIRI, activityIRI string) error {
		if actorIRI == "https://local-domain.com/actor/alice" && activityIRI == "https://remote.com/act/1" {
			deliveryRecorded = true
		}
		return nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
		return []model.Quad{}, nil
	})

	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/1",
		ObjectIRI:   "https://remote.com/note/1",
		Payload:     []byte(`{"to":["https://local-domain.com/actor/alice", "https://remote-domain.com/actor/bob"]}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if !deliveryRecorded {
		t.Errorf("Expected delivery recorded for alice via shared inbox fan-out")
	}
}

func TestProcessInboundTask_DirectInboxResolution(t *testing.T) {
	mc := minimock.NewController(t)

	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.SaveGraphVersionMock.Set(func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
		return nil
	})
	mockStorage.StreamQuadsBySubjectMock.Return([]model.Quad{}, nil)

	// Setup GetActorDualKeys expectations for resolving local actors
	mockStorage.GetActorDualKeysMock.Set(func(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
		if actorIRI == "https://local-domain.com/actor/alice" {
			return &model.ActorDualKeys{}, nil
		}
		return nil, errors.New("not local actor")
	})

	// Setup RecordActorInboxDelivery expectations
	deliveryRecorded := false
	mockStorage.RecordActorInboxDeliveryMock.Set(func(ctx context.Context, actorIRI, activityIRI string) error {
		if actorIRI == "https://local-domain.com/actor/alice" && activityIRI == "https://remote.com/act/1" {
			deliveryRecorded = true
		}
		return nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
		return []model.Quad{}, nil
	})

	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})

	// Direct Delivery task has objectIRI matching local actor profile.
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/1",
		ObjectIRI:   "https://local-domain.com/actor/alice",
		Payload:     []byte(`{}`), // Empty payload addressing but direct inbox targets objectIRI
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if !deliveryRecorded {
		t.Errorf("Expected delivery recorded for alice via direct inbox task.ObjectIRI mapping")
	}
}

func TestProcessInboundTask_StorageError(t *testing.T) {
	mc := minimock.NewController(t)

	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.SaveGraphVersionMock.Set(func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
		return errors.New("db error")
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
		return []model.Quad{}, nil
	})

	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{ID: "task-1"}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error on storage failure, got nil")
	}
}

func TestProcessInboundTask_ParserError(t *testing.T) {
	mc := minimock.NewController(t)

	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
		return nil, errors.New("parse error")
	})

	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{ID: "task-1"}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error on parser failure, got nil")
	}
}

func TestGetFollowersTimeline(t *testing.T) {
	mc := minimock.NewController(t)

	ctx := context.Background()
	actorIRI := "https://sprezz.net/actor/alice"
	followerPredicate := "https://www.w3.org/ns/activitystreams#follower"

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI != actorIRI {
			return nil, nil
		}
		return []model.Quad{
			{Subject: actorIRI, Predicate: followerPredicate, Object: "https://remote.com/users/bob"},
			{Subject: actorIRI, Predicate: followerPredicate, Object: "https://remote.com/users/charlie"},
			{Subject: actorIRI, Predicate: followerPredicate, Object: "https://remote.com/users/dave"},
			{Subject: actorIRI, Predicate: "https://schema.org/name", Object: "Alice"},
		}, nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})

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
	mc := minimock.NewController(t)

	ctx := context.Background()
	actorIRI := "https://sprezz.net/alice"
	readerBob := "https://remote.com/bob"

	publicPayload := []byte(`{"id":"https://sprezz.net/alice","type":"Create","to":["https://www.w3.org/ns/activitystreams#Public"]}`)
	privatePayload := []byte(`{"id":"https://sprezz.net/alice","type":"Create","to":["https://remote.com/bob"]}`)
	blockedPayload := []byte(`{"id":"https://sprezz.net/alice","type":"Create","to":["https://remote.com/dave"]}`)

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.GetCollectionPayloadsMock.Set(func(ctx context.Context, a, c string, l, o int) ([][]byte, error) {
		if a == actorIRI && c == "outbox" {
			return [][]byte{publicPayload, privatePayload, blockedPayload}, nil
		}
		return nil, nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
		if string(rawJSON) == string(publicPayload) {
			return []model.Quad{{GraphID: graphID, Subject: "act/1", Predicate: model.PredicateTo, Object: "https://www.w3.org/ns/activitystreams#Public", ObjType: model.NamedNode}}, nil
		}
		if string(rawJSON) == string(privatePayload) {
			return []model.Quad{{GraphID: graphID, Subject: "act/2", Predicate: model.PredicateTo, Object: readerBob, ObjType: model.NamedNode}}, nil
		}
		return []model.Quad{{GraphID: graphID, Subject: "act/3", Predicate: model.PredicateTo, Object: "https://remote.com", ObjType: model.NamedNode}}, nil
	})

	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})

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
	mc := minimock.NewController(t)

	ctx := context.Background()
	actorIRI := "https://example.com"

	// Mock existing seed private/public credentials configuration setup
	seedKeys, err := cryptoutil.MintNewKeyPair()
	if err != nil {
		t.Fatalf("failed setup: %v", err)
	}

	archiveCount := 0
	overwriteInvoked := false

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.GetActorCredentialsMock.Set(func(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error) {
		return actorIRI, &model.ActorDualKeys{
			PrivateKeyRSAPEM:     seedKeys.RSAPrivatePEM,
			PrivateKeyEd25519PEM: seedKeys.Ed25519PrivatePEM,
		}, nil
	})
	mockStorage.ArchiveKeyHistoryMock.Set(func(ctx context.Context, targetIRI string, keyType string, pubKeyPEM string, validFrom, validTo time.Time) error {
		archiveCount++
		return nil
	})
	mockStorage.CreateActorCredentialMock.Set(func(ctx context.Context, targetIRI string, tenantID int32, username string, rsaPEM, edPEM string) error {
		overwriteInvoked = true
		if rsaPEM == seedKeys.RSAPrivatePEM {
			t.Error("rotation failed to overwrite legacy private key with fresh material")
		}
		return nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})

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

func TestProcessInboundMediaTask_QuotaSuccess(t *testing.T) {
	ctx := context.Background()
	mc := minimock.NewController(t)

	var tenantID int32 = 1
	var fileSize int64 = 500

	mockStorage := portmock.NewStoragePortMock(mc)
	// Expect the quota pre-flight validation check to pass cleanly
	mockStorage.VerifyIncomingQuotaMock.Expect(ctx, tenantID, fileSize).Return(true, nil)

	// Stub the transactional graph writer mechanism
	mockWriter := portmock.NewGraphVersionWriterMock(mc)
	mockWriter.SaveGraphVersionWithMediaMock.Set(func(ctx context.Context, params port.MediaAttachmentParams) error {
		return nil
	})

	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockMedia.PutObjectMock.Return("stable-key", "sha256-hex", nil)

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Return([]model.Quad{}, nil)

	structMock := struct {
		*portmock.StoragePortMock
		*portmock.GraphVersionWriterMock
	}{mockStorage, mockWriter}

	svc := service.NewActivityService(structMock, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})

	mediaCtx := port.InboundMediaContext{
		ObjectName:  "tmp/task-abc",
		Size:        fileSize,
		MediaStream: strings.NewReader("fake-data"),
	}
	task := model.InboundTask{Payload: []byte(`{}`)}

	err := svc.ProcessInboundMediaTask(ctx, mediaCtx, task)
	if err != nil {
		t.Fatalf("Expected quota allocation confirmation to pass successfully, got error: %v", err)
	}
}

func TestProcessInboundMediaTask_QuotaBreached(t *testing.T) {
	ctx := context.Background()
	mc := minimock.NewController(t)

	var tenantID int32 = 1
	var fileSize int64 = 99999999

	mockStorage := portmock.NewStoragePortMock(mc)
	// Simulate a strict hard ceiling threshold limit breach event
	mockStorage.VerifyIncomingQuotaMock.Expect(ctx, tenantID, fileSize).Return(false, nil)

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})

	mediaCtx := port.InboundMediaContext{
		ObjectName:  "tmp/oversized-task",
		Size:        fileSize,
		MediaStream: strings.NewReader("massive-data"),
	}
	task := model.InboundTask{Payload: []byte(`{}`)}

	err := svc.ProcessInboundMediaTask(ctx, mediaCtx, task)
	if err == nil {
		t.Fatal("Expected pre-flight processing loop to intercept the oversized allocation, but got nil")
	}

	if !strings.Contains(err.Error(), "storage authorization ceiling threshold exceeded") {
		t.Errorf("Unexpected error message bubble surfaced from core service: %v", err)
	}
}

func TestDispatchOutboundActivity_SharedInboxConsolidation(t *testing.T) {
	ctx := context.Background()
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStoragePortMock(mc)

	mockStorage.GetTenantIDByDomainMock.Set(func(ctx context.Context, domain string) (int32, error) {
		return 1, nil
	})
	mockStorage.GetTenantIDByDomainMock.Set(func(ctx context.Context, domain string) (int32, error) {
		return 1, nil
	})
	mockStorage.GetActorCredentialsMock.Set(func(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error) {
		return "https://local.com/actor/server", &model.ActorDualKeys{
			PrivateKeyRSAPEM: "-----BEGIN RSA PRIVATE KEY-----",
		}, nil
	})

	// Stub dual key lookup for local sender
	mockStorage.GetActorDualKeysMock.Set(func(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
		if actorIRI == "https://local.com/actor/alice" {
			return &model.ActorDualKeys{
				PrivateKeyRSAPEM:     "-----BEGIN RSA PRIVATE KEY-----",
				PrivateKeyEd25519PEM: "-----BEGIN PRIVATE KEY-----",
			}, nil
		}
		return nil, errors.New("remote actor")
	})

	// Stub remote actor profiles lookup
	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com" {
			return []model.Quad{
				{Subject: subjectIRI, Predicate: model.PredicateSharedInbox, Object: "https://remote.com/inbox"},
			}, nil
		}
		if subjectIRI == "https://remote.com/actor/bob" {
			return []model.Quad{
				{Subject: subjectIRI, Predicate: model.PredicateSharedInbox, Object: "https://remote.com/inbox"},
				{Subject: subjectIRI, Predicate: model.PredicateInbox, Object: "https://remote.com/actor/bob/inbox"},
			}, nil
		}
		if subjectIRI == "https://remote.com/actor/charlie" {
			return []model.Quad{
				{Subject: subjectIRI, Predicate: model.PredicateSharedInbox, Object: "https://remote.com/inbox"},
				{Subject: subjectIRI, Predicate: model.PredicateInbox, Object: "https://remote.com/actor/charlie/inbox"},
			}, nil
		}
		if subjectIRI == "https://other.com/actor/dan" {
			return []model.Quad{
				{Subject: subjectIRI, Predicate: model.PredicateInbox, Object: "https://other.com/actor/dan/inbox"},
			}, nil
		}
		return nil, errors.New("not found")
	})

	// Expect exactly two signed POST requests: one to the consolidated shared inbox and one to the direct inbox of dan
	dispatchedInboxes := make(map[string]int)
	mockDispatcher := portmock.NewOutboundDispatcherMock(mc)
	mockDispatcher.ForwardFederatedActivityMock.Set(func(ctx context.Context, targetInbox, actorKeyID, rsaPEM, edPEM string, payload []byte) error {
		dispatchedInboxes[targetInbox]++
		return nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{}, mockDispatcher)

	payload := []byte(`{
		"to": ["https://remote.com/actor/bob", "https://remote.com/actor/charlie"],
		"cc": "https://other.com/actor/dan"
	}`)

	err := svc.DispatchOutboundActivity(ctx, "https://local.com/activity/1", "https://local.com/actor/alice", payload)
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}

	if len(dispatchedInboxes) != 2 {
		t.Errorf("expected exactly 2 outbound target endpoints dispatched, got: %v", dispatchedInboxes)
	}
	if dispatchedInboxes["https://remote.com/inbox"] != 1 {
		t.Errorf("expected exactly 1 dispatch to shared inbox, got: %d", dispatchedInboxes["https://remote.com/inbox"])
	}
	if dispatchedInboxes["https://other.com/actor/dan/inbox"] != 1 {
		t.Errorf("expected exactly 1 dispatch to direct inbox, got: %d", dispatchedInboxes["https://other.com/actor/dan/inbox"])
	}
}

func startDiscoveryMockServer(remoteHost *string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "webfinger") {
			w.Header().Set("Content-Type", "application/jrd+json")
			_, _ = w.Write([]byte(`{
				"links": [
					{
						"rel": "self",
						"type": "application/activity+json",
						"href": "http://` + *remoteHost + `/actor"
					}
				]
			}`))
			return
		}
		if r.URL.Path == "/actor" {
			w.Header().Set("Content-Type", "application/activity+json")
			_, _ = w.Write([]byte(`{
				"id": "http://` + *remoteHost + `/actor",
				"endpoints": {
					"sharedInbox": "http://` + *remoteHost + `/inbox"
				}
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestDispatchOutboundActivity_FEPD556Discovery(t *testing.T) {
	ctx := context.Background()
	mc := minimock.NewController(t)

	var remoteHost string

	// Spin up a mock remote server to reply to WebFinger and Actor profile GET requests
	server := startDiscoveryMockServer(&remoteHost)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	remoteHost = u.Host // e.g. 127.0.0.1:xxxxx

	mockStorage := portmock.NewStoragePortMock(mc)

	mockStorage.GetTenantIDByDomainMock.Set(func(ctx context.Context, domain string) (int32, error) {
		return 1, nil
	})

	// Generate real RSA key pair for cryptographic signing tests
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	privDER := x509.MarshalPKCS1PrivateKey(privKey)
	privBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privDER,
	}
	realRSAPEM := string(pem.EncodeToMemory(&privBlock))

	mockStorage.GetActorCredentialsMock.Set(func(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error) {
		return "https://local.com/actor/server", &model.ActorDualKeys{
			PrivateKeyRSAPEM: realRSAPEM,
		}, nil
	})

	mockStorage.GetActorDualKeysMock.Set(func(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
		if actorIRI == "https://local.com/actor/alice" {
			return &model.ActorDualKeys{
				PrivateKeyRSAPEM:     realRSAPEM,
				PrivateKeyEd25519PEM: "-----BEGIN PRIVATE KEY-----",
			}, nil
		}
		return nil, errors.New("remote actor")
	})

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		// Return not found so it triggers discovery!
		return nil, errors.New("not found")
	})

	createdGraph := false
	mockStorage.CreateGraphVersionMock.Set(func(ctx context.Context, activityIRI, objectIRI string, payload []byte) (int64, error) {
		createdGraph = true
		return 123, nil
	})

	savedQuads := false
	mockStorage.SaveQuadsMock.Set(func(ctx context.Context, quads []model.Quad) error {
		if len(quads) > 0 && quads[0].Subject == "https://"+remoteHost && quads[0].Object == "http://"+remoteHost+"/inbox" {
			savedQuads = true
		}
		return nil
	})

	mockDispatcher := portmock.NewOutboundDispatcherMock(mc)
	mockDispatcher.ForwardFederatedActivityMock.Set(func(ctx context.Context, targetInbox, actorKeyID, rsaPEM, edPEM string, payload []byte) error {
		return nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{}, mockDispatcher)

	payload := []byte(`{
		"to": ["https://` + remoteHost + `/actor/bob"]
	}`)

	err := svc.DispatchOutboundActivity(ctx, "https://local.com/activity/1", "https://local.com/actor/alice", payload)
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}

	if !createdGraph {
		t.Errorf("expected graph version to be created for discovered server actor")
	}
	if !savedQuads {
		t.Errorf("expected discovered sharedInbox quads to be saved to database")
	}
}

func setupStreamQuadsMockForInboxForwarding(mockStorage *portmock.StoragePortMock) {
	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://cached-relationship.com" {
			return []model.Quad{
				{Subject: subjectIRI, Predicate: model.PredicateSharedInbox, Object: "https://cached-relationship.com/inbox"},
			}, nil
		}
		if subjectIRI == "https://local.com/actor/alice" {
			return []model.Quad{
				{Subject: subjectIRI, Predicate: model.PredicateFollower, Object: "https://followers-relationship.com/actor/bob"},
			}, nil
		}
		if subjectIRI == "https://followers-relationship.com/actor/charlie" {
			return []model.Quad{
				{Subject: subjectIRI, Predicate: model.PredicateInbox, Object: "https://followers-relationship.com/actor/charlie/inbox"},
			}, nil
		}
		return nil, errors.New("not found")
	})
}

func TestProcessInboundTask_InboxForwarding(t *testing.T) {
	ctx := context.Background()
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStoragePortMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockDispatcher := portmock.NewOutboundDispatcherMock(mc)

	// Mock Storage Setups
	mockStorage.GetTenantIDByDomainMock.Set(func(ctx context.Context, domain string) (int32, error) {
		return 1, nil
	})
	mockStorage.GetActorCredentialsMock.Set(func(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error) {
		return "https://local.com/actor/server", &model.ActorDualKeys{
			PrivateKeyRSAPEM: "-----BEGIN RSA PRIVATE KEY-----",
		}, nil
	})

	mockStorage.GetActorDualKeysMock.Set(func(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
		if actorIRI == "https://local.com/actor/alice" {
			return &model.ActorDualKeys{
				PrivateKeyRSAPEM: "-----BEGIN RSA PRIVATE KEY-----",
			}, nil
		}
		return nil, errors.New("remote actor")
	})

	mockStorage.IsDomainBlockedMock.Set(func(ctx context.Context, domainName string) (bool, error) {
		if domainName == "blocked.com" {
			return true, nil
		}
		return false, nil
	})

	setupStreamQuadsMockForInboxForwarding(mockStorage)

	mockStorage.CreateGraphVersionMock.Set(func(ctx context.Context, activityIRI, objectIRI string, payload []byte) (int64, error) {
		return 1, nil
	})
	mockStorage.SaveQuadsMock.Set(func(ctx context.Context, quads []model.Quad) error {
		return nil
	})
	mockStorage.RecordActorInboxDeliveryMock.Set(func(ctx context.Context, actorIRI, activityIRI string) error {
		return nil
	})

	// Parser mock to return some quads
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, subjectIRI string, payload []byte) ([]model.Quad, error) {
		return []model.Quad{
			{Subject: subjectIRI, Predicate: "type", Object: "Note"},
		}, nil
	})

	// Outbound Dispatcher setup to track forwarded inboxes
	forwardedInboxes := make(map[string]int)
	mockDispatcher.ForwardFederatedActivityMock.Set(func(ctx context.Context, targetInbox, actorKeyID, rsaPEM, edPEM string, payload []byte) error {
		forwardedInboxes[targetInbox]++
		return nil
	})

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{}, mockDispatcher)

	// Activity payload addressing a local actor (alice),
	// a remote actor with a cached relationship,
	// a remote actor with relationship via followers,
	// a remote actor with no relationship, and a blocked domain actor.
	payload := []byte(`{
		"id": "https://remote.com/activity/1",
		"type": "Create",
		"actor": "https://remote.com/actor/original-author",
		"to": [
			"https://local.com/actor/alice",
			"https://cached-relationship.com/actor/bob",
			"https://followers-relationship.com/actor/charlie",
			"https://no-relationship.com/actor/dan",
			"https://blocked.com/actor/eve"
		]
	}`)

	task := model.InboundTask{
		ActivityIRI: "https://remote.com/activity/1",
		ObjectIRI:   "https://local.com/actor/alice",
		Payload:     payload,
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("expected ProcessInboundTask to succeed, got error: %v", err)
	}

	// Verify forwarding targets
	if forwardedInboxes["https://cached-relationship.com/inbox"] != 1 {
		t.Errorf("expected 1 forward to cached relationship domain, got: %d", forwardedInboxes["https://cached-relationship.com/inbox"])
	}

	// For followers-relationship.com, it will fallback to resolving individual direct inbox
	// because s.resolveServerActorInbox doesn't find a cached/resolved sharedInbox in StreamQuadsBySubject for that domain
	// and discoverRemoteActorIRI isn't fully stubbed to succeed (so it falls back to resolving direct actor inbox).
	if forwardedInboxes["https://followers-relationship.com/actor/charlie/inbox"] != 1 {
		t.Errorf("expected 1 forward to followers relationship domain direct inbox, got: %d", forwardedInboxes["https://followers-relationship.com/actor/charlie/inbox"])
	}

	// No-relationship should not be forwarded
	for inbox := range forwardedInboxes {
		if strings.Contains(inbox, "no-relationship.com") {
			t.Errorf("should not have forwarded to no-relationship domain: %s", inbox)
		}
		if strings.Contains(inbox, "blocked.com") {
			t.Errorf("should not have forwarded to blocked domain: %s", inbox)
		}
	}
}

func TestProcessInboundTask_GroupJoin_Success(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	groupIRI := "https://local.com/actor/group-1"
	senderIRI := "https://remote.com/actor/bob"

	savedQuads := false
	mockStorage := setupGroupStorageMock(mc, groupIRI, senderIRI, &savedQuads)

	dispatchedAccept := false
	mockDispatcher := portmock.NewOutboundDispatcherMock(mc)
	mockDispatcher.ForwardFederatedActivityMock.Set(func(ctx context.Context, targetInbox, actorKeyID, rsaPEM, edPEM string, payload []byte) error {
		var activity map[string]interface{}
		_ = json.Unmarshal(payload, &activity)
		if activity["type"] == "Accept" && targetInbox == "https://remote.com/actor/bob/inbox" {
			dispatchedAccept = true
		}
		return nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Return([]model.Quad{}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{}, mockDispatcher)

	payload := []byte(`{
		"id": "https://remote.com/activity/join-1",
		"type": "Join",
		"actor": "https://remote.com/actor/bob",
		"object": "https://local.com/actor/group-1"
	}`)

	task := model.InboundTask{
		ActivityIRI: "https://remote.com/activity/join-1",
		ObjectIRI:   groupIRI,
		Payload:     payload,
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected success, got err: %v", err)
	}

	if !savedQuads {
		t.Error("Expected group follower relationship to be saved in database")
	}
	if !dispatchedAccept {
		t.Error("Expected Accept(Join) activity to be dispatched outbound")
	}
}

func TestProcessInboundTask_GroupLeave_Success(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	groupIRI := "https://local.com/actor/group-1"
	senderIRI := "https://remote.com/actor/bob"

	mockStorage := setupGroupStorageMock(mc, groupIRI, senderIRI, nil)

	// Expect RemoveQuadEdge to be called to prune relationship
	removedEdge := false
	mockStorage.RemoveQuadEdgeMock.Set(func(ctx context.Context, subject, predicate, object string) error {
		if subject == groupIRI && predicate == model.PredicateFollower && object == senderIRI {
			removedEdge = true
		}
		return nil
	})

	dispatchedAccept := false
	mockDispatcher := portmock.NewOutboundDispatcherMock(mc)
	mockDispatcher.ForwardFederatedActivityMock.Set(func(ctx context.Context, targetInbox, actorKeyID, rsaPEM, edPEM string, payload []byte) error {
		var activity map[string]interface{}
		_ = json.Unmarshal(payload, &activity)
		if activity["type"] == "Accept" && targetInbox == "https://remote.com/actor/bob/inbox" {
			dispatchedAccept = true
		}
		return nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Return([]model.Quad{}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{}, mockDispatcher)

	payload := []byte(`{
		"id": "https://remote.com/activity/leave-1",
		"type": "Leave",
		"actor": "https://remote.com/actor/bob",
		"object": "https://local.com/actor/group-1"
	}`)

	task := model.InboundTask{
		ActivityIRI: "https://remote.com/activity/leave-1",
		ObjectIRI:   groupIRI,
		Payload:     payload,
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected success, got err: %v", err)
	}

	if !removedEdge {
		t.Error("Expected group follower relationship to be removed from database")
	}
	if !dispatchedAccept {
		t.Error("Expected Accept(Leave) activity to be dispatched outbound")
	}
}

func TestProcessInboundTask_GroupAnnounce_Success(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	groupIRI := "https://local.com/actor/group-1"
	senderIRI := "https://remote.com/actor/bob"

	mockStorage := setupGroupStorageMock(mc, groupIRI, senderIRI, nil)
	mockStorage.RecordActorInboxDeliveryMock.Return(nil)

	// Bob is a member! Group quads should reflect that
	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subject string) ([]model.Quad, error) {
		if subject == groupIRI {
			return []model.Quad{
				{Subject: groupIRI, Predicate: model.RDFType, Object: model.ActorGroup},
				{Subject: groupIRI, Predicate: model.PredicateFollower, Object: senderIRI},
				{Subject: groupIRI, Predicate: model.PredicateFollower, Object: "https://remote-2.com/actor/charlie"},
			}, nil
		}
		if subject == "https://remote-2.com/actor/charlie" {
			return []model.Quad{
				{Subject: "https://remote-2.com/actor/charlie", Predicate: model.PredicateInbox, Object: "https://remote-2.com/actor/charlie/inbox"},
			}, nil
		}
		return nil, nil
	})

	dispatchedAnnounce := false
	mockDispatcher := portmock.NewOutboundDispatcherMock(mc)
	mockDispatcher.ForwardFederatedActivityMock.Set(func(ctx context.Context, targetInbox, actorKeyID, rsaPEM, edPEM string, payload []byte) error {
		var activity map[string]interface{}
		_ = json.Unmarshal(payload, &activity)
		if activity["type"] == "Announce" && targetInbox == "https://remote-2.com/actor/charlie/inbox" {
			dispatchedAnnounce = true
		}
		return nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Return([]model.Quad{}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{}, mockDispatcher)

	payload := []byte(`{
		"id": "https://remote.com/activity/create-1",
		"type": "Create",
		"actor": "https://remote.com/actor/bob",
		"object": "https://remote.com/note/123"
	}`)

	task := model.InboundTask{
		ActivityIRI: "https://remote.com/activity/create-1",
		ObjectIRI:   groupIRI,
		Payload:     payload,
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected success, got err: %v", err)
	}

	if !dispatchedAnnounce {
		t.Error("Expected inbound post to be auto-announced out to group members")
	}
}

func TestProcessInboundTask_GroupAnnounce_NonMemberRejected(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	groupIRI := "https://local.com/actor/group-1"
	senderIRI := "https://remote.com/actor/bob"

	mockStorage := setupGroupStorageMock(mc, groupIRI, senderIRI, nil)

	// Bob is NOT a member (no follower quad)!
	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subject string) ([]model.Quad, error) {
		if subject == groupIRI {
			return []model.Quad{
				{Subject: groupIRI, Predicate: model.RDFType, Object: model.ActorGroup},
			}, nil
		}
		return nil, nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Return([]model.Quad{}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})

	payload := []byte(`{
		"id": "https://remote.com/activity/create-1",
		"type": "Create",
		"actor": "https://remote.com/actor/bob",
		"object": "https://remote.com/note/123"
	}`)

	task := model.InboundTask{
		ActivityIRI: "https://remote.com/activity/create-1",
		ObjectIRI:   groupIRI,
		Payload:     payload,
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatal("Expected error when non-member attempts to post to Group, got nil")
	}
	if !strings.Contains(err.Error(), "sender https://remote.com/actor/bob is not a member of group") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func setupGroupStorageMock(mc *minimock.Controller, groupIRI, senderIRI string, savedQuads *bool) *portmock.StoragePortMock {
	mockStorage := portmock.NewStoragePortMock(mc)
	mockStorage.CreateGraphVersionMock.Return(int64(123), nil)

	if savedQuads != nil {
		mockStorage.SaveQuadsMock.Set(func(ctx context.Context, quads []model.Quad) error {
			if len(quads) > 0 && quads[0].Subject == groupIRI {
				*savedQuads = true
			}
			return nil
		})
	} else {
		mockStorage.SaveQuadsMock.Return(nil)
	}

	mockStorage.GetTenantIDByDomainMock.Optional().Return(int32(1), nil)
	mockStorage.GetActorCredentialsMock.Optional().Return("https://local.com/actor/server", &model.ActorDualKeys{PrivateKeyRSAPEM: "server-rsa-private"}, nil)

	mockStorage.GetActorDualKeysMock.Set(func(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
		if actorIRI == groupIRI {
			return &model.ActorDualKeys{
				PrivateKeyRSAPEM:     "server-rsa-private",
				PrivateKeyEd25519PEM: "server-ed-private",
			}, nil
		}
		return nil, errors.New("not local")
	})

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subject string) ([]model.Quad, error) {
		if subject == groupIRI {
			return []model.Quad{
				{Subject: groupIRI, Predicate: model.RDFType, Object: model.ActorGroup},
			}, nil
		}
		if subject == senderIRI {
			return []model.Quad{
				{Subject: senderIRI, Predicate: model.PredicateInbox, Object: "https://remote.com/actor/bob/inbox"},
			}, nil
		}
		return nil, nil
	})

	return mockStorage
}

func TestProcessInboundTask_ActorSpoofing_PathAgnostic(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia, createTestFetcher(mc), service.ActivityServiceConfig{})

	// Target object has an arbitrary path IRI (no '/actor/' segment), but its type is "Person" (Actor type)
	// Bob and Alice are both on "remote.com" (domains match, bypassing domain check), but Bob is not authorized to create Alice's profile.
	payload := []byte(`{
		"id": "https://remote.com/activity/create-alice",
		"type": "Create",
		"actor": "https://remote.com/actor/bob",
		"object": {
			"id": "https://remote.com/@alice",
			"type": "Person",
			"preferredUsername": "alice"
		}
	}`)

	task := model.InboundTask{
		ID:          "task-1",
		ActivityIRI: "https://remote.com/activity/create-alice",
		ObjectIRI:   "https://remote.com/@alice",
		Payload:     payload,
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatal("Expected security violation error, got nil")
	}
	if !strings.Contains(err.Error(), "is not authorized to create actor profile") {
		t.Errorf("Unexpected error message: %v", err)
	}
}
