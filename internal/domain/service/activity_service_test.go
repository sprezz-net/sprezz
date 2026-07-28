package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/domain/service"
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

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
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

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
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

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)

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

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
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

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
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

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)

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
			return []model.Quad{{GraphID: graphID, Subject: "act/1", Predicate: "activitystreams#to", Object: "https://www.w3.org/ns/activitystreams#Public", ObjType: model.NamedNode}}, nil
		}
		if string(rawJSON) == string(privatePayload) {
			return []model.Quad{{GraphID: graphID, Subject: "act/2", Predicate: "activitystreams#to", Object: readerBob, ObjType: model.NamedNode}}, nil
		}
		return []model.Quad{{GraphID: graphID, Subject: "act/3", Predicate: "activitystreams#to", Object: "https://remote.com", ObjType: model.NamedNode}}, nil
	})

	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)

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
	seedKeys, err := model.MintNewKeyPair()
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

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)

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

	svc := service.NewActivityService(structMock, mockParser, mockMedia)

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

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc))

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
		if subjectIRI == "https://remote.com/actor/bob" {
			return []model.Quad{
				{Subject: subjectIRI, Predicate: "activitystreams#sharedInbox", Object: "https://remote.com/inbox"},
				{Subject: subjectIRI, Predicate: "activitystreams#inbox", Object: "https://remote.com/actor/bob/inbox"},
			}, nil
		}
		if subjectIRI == "https://remote.com/actor/charlie" {
			return []model.Quad{
				{Subject: subjectIRI, Predicate: "activitystreams#sharedInbox", Object: "https://remote.com/inbox"},
				{Subject: subjectIRI, Predicate: "activitystreams#inbox", Object: "https://remote.com/actor/charlie/inbox"},
			}, nil
		}
		if subjectIRI == "https://other.com/actor/dan" {
			return []model.Quad{
				{Subject: subjectIRI, Predicate: "activitystreams#inbox", Object: "https://other.com/actor/dan/inbox"},
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

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), mockDispatcher)

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
