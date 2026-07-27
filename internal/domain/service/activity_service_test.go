package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"sprezz/internal/domain/model"
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
	actorIRI := "https://sprezz.net/actors/alice"
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
