package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/domain/service"
)

func TestActivityService_AcceptFollow_Success(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockForwarder := portmock.NewOutboundDispatcherMock(mc)

	followActivityIRI := "https://remote.com/act/follow-123"
	followedActorIRI := "https://local.com/actor/alice"
	followerActorIRI := "https://remote.com/actor/bob"

	followPayload := []byte(`{"type":"Follow","actor":"https://remote.com/actor/bob","object":"https://local.com/actor/alice"}`)
	mockStorage.GetLatestPayloadMock.Expect(ctx, followActivityIRI).Return(followPayload, nil)

	setupGetActorDualKeysMock(mockStorage, followedActorIRI)
	setupForwardFederatedActivityMock(t, mockForwarder, "Accept", followedActorIRI, followerActorIRI)
	setupSaveQuadsMockAccept(t, mockStorage, followActivityIRI, followedActorIRI, followerActorIRI)
	setupStreamQuadsBySubjectMock(mockStorage, followActivityIRI, followerActorIRI, followedActorIRI)

	mockStorage.GetTenantIDByDomainMock.Return(1, nil)
	mockStorage.GetActorCredentialsMock.Return("https://local.com/actor/server", &model.ActorDualKeys{
		PrivateKeyRSAPEM:     "server-rsa-private",
		PrivateKeyEd25519PEM: "server-ed-private",
	}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), mockForwarder)

	err := svc.AcceptFollow(ctx, followedActorIRI, followActivityIRI)
	if err != nil {
		t.Fatalf("Expected AcceptFollow success, got error: %v", err)
	}
}

func TestActivityService_RejectFollow_Success(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockForwarder := portmock.NewOutboundDispatcherMock(mc)

	followActivityIRI := "https://remote.com/act/follow-123"
	followedActorIRI := "https://local.com/actor/alice"
	followerActorIRI := "https://remote.com/actor/bob"

	followPayload := []byte(`{"type":"Follow","actor":"https://remote.com/actor/bob","object":"https://local.com/actor/alice"}`)
	mockStorage.GetLatestPayloadMock.Expect(ctx, followActivityIRI).Return(followPayload, nil)

	setupGetActorDualKeysMock(mockStorage, followedActorIRI)
	setupForwardFederatedActivityMock(t, mockForwarder, "Reject", followedActorIRI, followerActorIRI)
	setupSaveQuadsMockReject(t, mockStorage, followActivityIRI)
	setupStreamQuadsBySubjectMock(mockStorage, followActivityIRI, followerActorIRI, followedActorIRI)

	mockStorage.GetTenantIDByDomainMock.Return(1, nil)
	mockStorage.GetActorCredentialsMock.Return("https://local.com/actor/server", &model.ActorDualKeys{
		PrivateKeyRSAPEM:     "server-rsa-private",
		PrivateKeyEd25519PEM: "server-ed-private",
	}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), mockForwarder)

	err := svc.RejectFollow(ctx, followedActorIRI, followActivityIRI)
	if err != nil {
		t.Fatalf("Expected RejectFollow success, got error: %v", err)
	}
}

func TestActivityService_AcceptFollow_Mismatch_Error(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)

	followActivityIRI := "https://remote.com/act/follow-123"
	followedActorIRI := "https://local.com/actor/alice"

	// Mock Follow targeting bob, but alice is trying to accept it
	mockStorage.StreamQuadsBySubjectMock.Expect(ctx, followActivityIRI).Return([]model.Quad{
		{Subject: followActivityIRI, Predicate: "as:actor", Object: "https://remote.com/actor/bob"},
		{Subject: followActivityIRI, Predicate: "as:object", Object: "https://local.com/actor/charlie"}, // Mismatched!
	}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc))

	err := svc.AcceptFollow(ctx, followedActorIRI, followActivityIRI)
	if err == nil {
		t.Fatalf("Expected error due to mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "followed actor mismatch") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Shared Mock Configuration Helpers to reduce test Cognitive Complexity
// -----------------------------------------------------------------------------

func setupGetActorDualKeysMock(mockStorage *portmock.StorageAndGraphWriterMock, expectedActorIRI string) {
	mockStorage.GetActorDualKeysMock.Set(func(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
		if actorIRI == expectedActorIRI {
			return &model.ActorDualKeys{
				PrivateKeyRSAPEM:     "mock-rsa-private",
				PrivateKeyEd25519PEM: "mock-ed-private",
			}, nil
		}
		return nil, fmt.Errorf("not local")
	})
}

func assertDispatchedActivity(t *testing.T, payload []byte, expectedType, followedActorIRI, followerActorIRI string) {
	var activity map[string]interface{}
	if err := json.Unmarshal(payload, &activity); err != nil {
		t.Fatalf("Failed to parse dispatched activity: %v", err)
	}
	if activity["type"] != expectedType {
		t.Errorf("Expected activity type %s, got: %s", expectedType, activity["type"])
	}
	if expectedType == "Accept" {
		assertAcceptActivityObject(t, activity, followedActorIRI, followerActorIRI)
	}
}

func assertAcceptActivityObject(t *testing.T, activity map[string]interface{}, followedActorIRI, followerActorIRI string) {
	if activity["actor"] != followedActorIRI {
		t.Errorf("Expected actor %s, got: %s", followedActorIRI, activity["actor"])
	}
	object, ok := activity["object"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected activity object to be a JSON object")
	}
	if object["type"] != "Follow" || object["actor"] != followerActorIRI {
		t.Errorf("Incorrect wrapped follow object: %v", object)
	}
}

func setupForwardFederatedActivityMock(t *testing.T, mockForwarder *portmock.OutboundDispatcherMock, expectedType, followedActorIRI, followerActorIRI string) {
	mockForwarder.ForwardFederatedActivityMock.Set(func(ctx context.Context, inbox, keyID, privateKeyRSAPEM, privateKeyEd25519PEM string, payload []byte) error {
		assertDispatchedActivity(t, payload, expectedType, followedActorIRI, followerActorIRI)
		return nil
	})
}

func setupSaveQuadsMockAccept(t *testing.T, mockStorage *portmock.StorageAndGraphWriterMock, followActivityIRI, followedActorIRI, followerActorIRI string) {
	mockStorage.SaveQuadsMock.Set(func(ctx context.Context, quads []model.Quad) error {
		if len(quads) != 1 {
			t.Fatalf("Expected 1 quad saved, got %d", len(quads))
		}
		q := quads[0]
		switch q.Subject {
		case followActivityIRI:
			if q.Predicate != model.PredicateAccepted || q.Object != "true" {
				t.Errorf("Unexpected follow state quad saved: %v", q)
			}
		case followedActorIRI:
			if q.Predicate != model.PredicateFollower || q.Object != followerActorIRI {
				t.Errorf("Unexpected follower relationship quad saved: %v", q)
			}
		default:
			t.Errorf("Unexpected quad subject saved: %s", q.Subject)
		}
		return nil
	})
}

func setupSaveQuadsMockReject(t *testing.T, mockStorage *portmock.StorageAndGraphWriterMock, followActivityIRI string) {
	mockStorage.SaveQuadsMock.Set(func(ctx context.Context, quads []model.Quad) error {
		if len(quads) != 1 {
			t.Fatalf("Expected exactly 1 quad saved, got %d", len(quads))
		}
		q := quads[0]
		switch q.Subject {
		case followActivityIRI:
			if q.Predicate != model.PredicateRejected || q.Object != "true" {
				t.Errorf("Unexpected follow state quad saved: %v", q)
			}
		default:
			t.Errorf("Unexpected quad subject saved: %s", q.Subject)
		}
		return nil
	})
}

func setupStreamQuadsBySubjectMock(mockStorage *portmock.StorageAndGraphWriterMock, followActivityIRI, followerActorIRI, followedActorIRI string) {
	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == followActivityIRI {
			return []model.Quad{
				{Subject: followActivityIRI, Predicate: "as:actor", Object: followerActorIRI},
				{Subject: followActivityIRI, Predicate: "as:object", Object: followedActorIRI},
			}, nil
		}
		if subjectIRI == followerActorIRI {
			return []model.Quad{
				{Subject: followerActorIRI, Predicate: "https://www.w3.org/ns/activitystreams#inbox", Object: "https://remote.com/actor/bob/inbox"},
			}, nil
		}
		return nil, nil
	})
}

func createTestFetcher(mc *minimock.Controller) *portmock.RemoteFetcherMock {
	mockFetcher := portmock.NewRemoteFetcherMock(mc)
	mockFetcher.FetchSignedMock.Optional().Set(func(ctx context.Context, targetURL string, keyID string, privateKeyRSAPEM string, privateKeyEd25519PEM string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	})
	return mockFetcher
}
