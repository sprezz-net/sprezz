package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/domain/service"

	"github.com/gojuno/minimock/v3"
)

func TestProcessInboundTask_Create_Spoof(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://evil.com/act/create-1",
		ObjectIRI:   "https://trusted.com/note/1",
		Payload:     []byte(`{"type":"Create","actor":"https://evil.com/actor/mallory","object":"https://trusted.com/note/1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatalf("Expected security violation error due to domain spoofing, got nil")
	}
	if !strings.Contains(err.Error(), "security violation: actor domain evil.com does not match object origin domain trusted.com") {
		t.Errorf("Unexpected error, got: %v", err)
	}
}

func TestProcessInboundTask_AcceptReject_Success(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.SaveGraphVersionMock.Return(nil)
	mockStorage.GetActorDualKeysMock.Return(nil, errors.New("not local"))

	// Pending Follow activity has alice as actor and bob as object/target
	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/bob" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		if subjectIRI == "https://remote.com/act/follow-1" {
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateActor, Object: "https://remote.com/actor/alice"},
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateObject, Object: "https://remote.com/actor/bob"},
			}, nil
		}
		return nil, nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Return([]model.Quad{}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/accept-1",
		ObjectIRI:   "https://remote.com/act/follow-1",
		Payload:     []byte(`{"type":"Accept","actor":"https://remote.com/actor/bob","object":"https://remote.com/act/follow-1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
}

func TestProcessInboundTask_AcceptReject_Mismatch(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/mallory" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		if subjectIRI == "https://remote.com/act/follow-1" {
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateActor, Object: "https://remote.com/actor/alice"},
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateObject, Object: "https://remote.com/actor/bob"},
			}, nil
		}
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/accept-1",
		ObjectIRI:   "https://remote.com/act/follow-1",
		Payload:     []byte(`{"type":"Accept","actor":"https://remote.com/actor/mallory","object":"https://remote.com/act/follow-1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "is not authorized to Accept follow sent by https://remote.com/actor/alice") {
		t.Errorf("Unexpected error, got: %v", err)
	}
}

func TestProcessInboundTask_AcceptReject_NotPending(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/bob" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		if subjectIRI == "https://remote.com/act/follow-1" {
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateActor, Object: "https://remote.com/actor/alice"},
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateObject, Object: "https://remote.com/actor/bob"},
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateAccepted, Object: "true"}, // Already accepted state!
			}, nil
		}
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/accept-1",
		ObjectIRI:   "https://remote.com/act/follow-1",
		Payload:     []byte(`{"type":"Accept","actor":"https://remote.com/actor/bob","object":"https://remote.com/act/follow-1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "is not in a pending state") {
		t.Errorf("Unexpected error, got: %v", err)
	}
}

func TestProcessInboundTask_Like_PrivateClearanceFail(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/mallory" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		if subjectIRI == "https://remote.com/note/private-1" {
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateAttributedTo, Object: "https://remote.com/actor/alice"},
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateTo, Object: "https://remote.com/actor/bob"}, // Not mallory!
			}, nil
		}
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/like-1",
		ObjectIRI:   "https://remote.com/note/private-1",
		Payload:     []byte(`{"type":"Like","actor":"https://remote.com/actor/mallory","object":"https://remote.com/note/private-1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatalf("Expected clearance failure, got nil")
	}
	if !strings.Contains(err.Error(), "does not have privacy clearance to view private object") {
		t.Errorf("Unexpected error, got: %v", err)
	}
}

func TestProcessInboundTask_Like_Duplicate(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/bob" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
				{GraphID: 1, Subject: subjectIRI, Predicate: "https://www.w3.org/ns/activitystreams#liked", Object: "https://remote.com/note/1"}, // Already liked!
			}, nil
		}
		if subjectIRI == "https://remote.com/note/1" {
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateAttributedTo, Object: "https://remote.com/actor/alice"},
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateTo, Object: "https://www.w3.org/ns/activitystreams#Public"},
			}, nil
		}
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/like-1",
		ObjectIRI:   "https://remote.com/note/1",
		Payload:     []byte(`{"type":"Like","actor":"https://remote.com/actor/bob","object":"https://remote.com/note/1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatalf("Expected duplicate like/idempotency violation, got nil")
	}
	if !strings.Contains(err.Error(), "has already liked object") {
		t.Errorf("Unexpected error, got: %v", err)
	}
}

func TestProcessInboundTask_Announce_PrivateFail(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/bob" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		if subjectIRI == "https://remote.com/note/private-1" {
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateAttributedTo, Object: "https://remote.com/actor/alice"},
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateTo, Object: "https://remote.com/actor/bob"}, // Limited audience!
			}, nil
		}
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/announce-1",
		ObjectIRI:   "https://remote.com/note/private-1",
		Payload:     []byte(`{"type":"Announce","actor":"https://remote.com/actor/bob","object":"https://remote.com/note/private-1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatalf("Expected privacy guard failure, got nil")
	}
	if !strings.Contains(err.Error(), "cannot announce private/limited object") {
		t.Errorf("Unexpected error, got: %v", err)
	}
}

func TestProcessInboundTask_Announce_JIT_Fetch_PublicSuccess(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	// Spin up a mock remote server to serve the remote post profile during JIT fetch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/activity+json")
		_, _ = w.Write([]byte(`{
			"id": "https://remote-origin.com/note/1",
			"to": ["https://www.w3.org/ns/activitystreams#Public"]
		}`))
	}))
	defer server.Close()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.SaveGraphVersionMock.Return(nil)
	mockStorage.GetActorDualKeysMock.Return(nil, errors.New("not local"))

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/bob" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		// target post is uncached, return empty
		return []model.Quad{}, nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Return([]model.Quad{}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/announce-1",
		ObjectIRI:   server.URL, // Use the server URL so JIT fetch hits our mock server!
		Payload:     []byte(`{"type":"Announce","actor":"https://remote.com/actor/bob","object":"` + server.URL + `"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected success on public remote announce JIT fetch, got error: %v", err)
	}
}

func TestProcessInboundTask_Announce_JIT_Fetch_PrivateFail(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	// Spin up a mock remote server to serve a private remote post during JIT fetch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/activity+json")
		_, _ = w.Write([]byte(`{
			"id": "https://remote-origin.com/note/1",
			"to": ["https://remote-origin.com/users/followers"]
		}`))
	}))
	defer server.Close()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/bob" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		return []model.Quad{}, nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/announce-1",
		ObjectIRI:   server.URL,
		Payload:     []byte(`{"type":"Announce","actor":"https://remote.com/actor/bob","object":"` + server.URL + `"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatalf("Expected privacy guard failure, got nil")
	}
	if !strings.Contains(err.Error(), "privacy guard: cannot announce private/limited remote object") {
		t.Errorf("Unexpected error, got: %v", err)
	}
}

func TestProcessInboundTask_JoinLeave_GroupSuccess(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.SaveGraphVersionMock.Return(nil)
	mockStorage.GetActorDualKeysMock.Return(nil, errors.New("not local"))

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/alice" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		if subjectIRI == "https://remote.com/groups/tech" {
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.RDFType, Object: model.ActorGroup},
			}, nil
		}
		return nil, nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Return([]model.Quad{}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/join-1",
		ObjectIRI:   "https://remote.com/groups/tech",
		Payload:     []byte(`{"type":"Join","actor":"https://remote.com/actor/alice","object":"https://remote.com/groups/tech"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
}

func TestProcessInboundTask_JoinLeave_NotGroupFail(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/alice" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		if subjectIRI == "https://remote.com/note/1" {
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.RDFType, Object: model.TypeNote}, // Note, not Group!
			}, nil
		}
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/join-1",
		ObjectIRI:   "https://remote.com/note/1",
		Payload:     []byte(`{"type":"Join","actor":"https://remote.com/actor/alice","object":"https://remote.com/note/1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatalf("Expected group check failure, got nil")
	}
	if !strings.Contains(err.Error(), "target scoping violation:") {
		t.Errorf("Unexpected error, got: %v", err)
	}
}

func TestProcessInboundTask_Question_Expired(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/alice" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		if subjectIRI == "https://remote.com/poll/1" {
			pastTime := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateEndTime, Object: pastTime},
			}, nil
		}
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/vote-1",
		ObjectIRI:   "https://remote.com/poll/1",
		Payload:     []byte(`{"type":"Question","actor":"https://remote.com/actor/alice","object":"https://remote.com/poll/1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatalf("Expected expired poll error, got nil")
	}
	if !strings.Contains(err.Error(), "poll Question https://remote.com/poll/1 has already expired") {
		t.Errorf("Unexpected error, got: %v", err)
	}
}

func TestProcessInboundTask_Question_DoubleVote(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/alice" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		if subjectIRI == "https://remote.com/poll/1" {
			futureTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateEndTime, Object: futureTime},
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateVoted, Object: "https://remote.com/actor/alice"}, // Already voted!
			}, nil
		}
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/vote-1",
		ObjectIRI:   "https://remote.com/poll/1",
		Payload:     []byte(`{"type":"Question","actor":"https://remote.com/actor/alice","object":"https://remote.com/poll/1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err == nil {
		t.Fatalf("Expected double-vote error, got nil")
	}
	if !strings.Contains(err.Error(), "double-vote violation:") {
		t.Errorf("Unexpected error, got: %v", err)
	}
}

func TestProcessInboundTask_Question_UpdateVoteSuccess(t *testing.T) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.SaveGraphVersionMock.Return(nil)
	mockStorage.GetActorDualKeysMock.Return(nil, errors.New("not local"))
	mockStorage.GetTenantIDByActivityIRIMock.Return(1, nil)

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/actor/alice" {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		return nil, nil
	})

	mockStorage.GetStatementsBySubjectIsolatedMock.Set(func(ctx context.Context, subjectIRI string, tenantID int32) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/poll/1" {
			futureTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
			return []model.Quad{
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateEndTime, Object: futureTime},
				{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateVoted, Object: "https://remote.com/actor/alice"}, // Already voted!
			}, nil
		}
		return nil, nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Return([]model.Quad{}, nil)

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})
	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/vote-1",
		ObjectIRI:   "https://remote.com/poll/1",
		Payload:     []byte(`{"type":"Update","actor":"https://remote.com/actor/alice","object":"https://remote.com/poll/1"}`), // Explicitly an Update activity!
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected success for re-voting via Update activity, got error: %v", err)
	}
}

type typeConfusionTestCase struct {
	name          string
	payload       string
	expectedError string
}

func getTypeConfusionTestCases() []typeConfusionTestCase {
	return []typeConfusionTestCase{
		{
			name: "array of maps for actor",
			payload: `{
				"type": "Create",
				"actor": [{"id": "https://remote.com/actor/alice"}],
				"object": "https://remote.com/note/1"
			}`,
			expectedError: "", // should validate actor successfully and proceed
		},
		{
			name: "deeply nested @id and @value structure for object",
			payload: `{
				"type": "Create",
				"actor": "https://remote.com/actor/alice",
				"object": [{"@id": "https://remote.com/note/1"}]
			}`,
			expectedError: "", // should validate object successfully and proceed
		},
		{
			name: "nested domain spoofing block inside expanded structure",
			payload: `{
				"type": "Create",
				"actor": [{"id": "https://remote.com/actor/alice"}],
				"object": [{"@id": "https://trusted.com/note/1"}]
			}`,
			expectedError: "security violation: actor domain remote.com does not match object origin domain trusted.com",
		},
		{
			name: "array of strings for type validation",
			payload: `{
				"type": "Create",
				"actor": "https://remote.com/actor/alice",
				"object": {
					"id": "https://remote.com/actor/alice",
					"type": ["Person", "Actor"]
				}
			}`,
			expectedError: "", // Person/Actor matches profile. Self-creation checks should pass safely.
		},
		{
			name: "array of maps for to address list",
			payload: `{
				"type": "Create",
				"actor": "https://remote.com/actor/alice",
				"object": "https://remote.com/note/1",
				"to": [{"id": "https://www.w3.org/ns/activitystreams#Public"}]
			}`,
			expectedError: "", // Should successfully extract the Public audience without panic or crash
		},
	}
}

func runTypeConfusionTest(t *testing.T, ctx context.Context, payload, expectedError string) {
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)

	// Mark all mocks optional so subtests that exit early are not counted as failed uncalled expectations
	mockStorage.SaveGraphVersionMock.Optional()
	mockParser.ToQuadsMock.Optional()
	mockStorage.GetActorDualKeysMock.Optional()
	mockStorage.StreamQuadsBySubjectMock.Optional()

	if expectedError == "" {
		mockStorage.SaveGraphVersionMock.Return(nil)
		mockParser.ToQuadsMock.Return([]model.Quad{}, nil)
	}
	mockStorage.GetActorDualKeysMock.Return(nil, errors.New("not local"))

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if strings.HasPrefix(subjectIRI, "https://remote.com/actor") {
			return []model.Quad{
				{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
			}, nil
		}
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})

	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/create-1",
		ObjectIRI:   "https://remote.com/note/1",
		Payload:     []byte(payload),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if expectedError == "" {
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
	} else {
		if err == nil {
			t.Fatalf("Expected error %q, got nil", expectedError)
		}
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error containing %q, got: %v", expectedError, err)
		}
	}
}

func TestProcessInboundTask_TypeConfusion_Resilience(t *testing.T) {
	ctx := context.Background()
	tests := getTypeConfusionTestCases()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runTypeConfusionTest(t, ctx, tc.payload, tc.expectedError)
		})
	}
}

type addRemoveTestCase struct {
	name          string
	actorIRI      string
	collectionIRI string
	publicAppend  string // "true", "1", "false", or ""
	expectError   bool
	errContains   string
}

func getAddRemoveTestCases() []addRemoveTestCase {
	return []addRemoveTestCase{
		{
			name:          "Owner of collection can Add/Remove",
			actorIRI:      "https://remote.com/actor/alice",
			collectionIRI: "https://remote.com/collection/alice-list",
			publicAppend:  "",
			expectError:   false,
		},
		{
			name:          "Non-owner rejected if publicAppend is missing",
			actorIRI:      "https://remote.com/actor/bob",
			collectionIRI: "https://remote.com/collection/alice-list",
			publicAppend:  "",
			expectError:   true,
			errContains:   "security violation: actor https://remote.com/actor/bob is not authorized to edit collection",
		},
		{
			name:          "Non-owner rejected if publicAppend is false",
			actorIRI:      "https://remote.com/actor/bob",
			collectionIRI: "https://remote.com/collection/alice-list",
			publicAppend:  "false",
			expectError:   true,
			errContains:   "security violation: actor https://remote.com/actor/bob is not authorized to edit collection",
		},
		{
			name:          "Non-owner allowed if publicAppend is true",
			actorIRI:      "https://remote.com/actor/bob",
			collectionIRI: "https://remote.com/collection/alice-list",
			publicAppend:  "true",
			expectError:   false,
		},
		{
			name:          "Non-owner allowed if publicAppend is 1",
			actorIRI:      "https://remote.com/actor/bob",
			collectionIRI: "https://remote.com/collection/alice-list",
			publicAppend:  "1",
			expectError:   false,
		},
	}
}

func mockStreamQuadsBySubject(tc addRemoveTestCase, subjectIRI string) ([]model.Quad, error) {
	if subjectIRI == tc.actorIRI {
		return []model.Quad{
			{GraphID: 1, Subject: subjectIRI, Predicate: model.PredicatePublicKeyPem, Object: "mock-pubkey"},
		}, nil
	}
	if subjectIRI == tc.collectionIRI {
		quads := []model.Quad{
			{GraphID: 2, Subject: subjectIRI, Predicate: model.PredicateActor, Object: "https://remote.com/actor/alice"},
		}
		if tc.publicAppend != "" {
			quads = append(quads, model.Quad{
				GraphID:   2,
				Subject:   subjectIRI,
				Predicate: model.PredicatePublicAppend,
				Object:    tc.publicAppend,
			})
		}
		return quads, nil
	}
	return nil, nil
}

func runAddRemoveTestCase(t *testing.T, tc addRemoveTestCase) {
	mc := minimock.NewController(t)
	ctx := context.Background()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)

	mockStorage.SaveGraphVersionMock.Optional()
	mockParser.ToQuadsMock.Optional()
	mockStorage.GetActorDualKeysMock.Optional()
	mockStorage.StreamQuadsBySubjectMock.Optional()

	if !tc.expectError {
		mockStorage.SaveGraphVersionMock.Return(nil)
		mockParser.ToQuadsMock.Return([]model.Quad{}, nil)
	}
	mockStorage.GetActorDualKeysMock.Return(nil, errors.New("not local"))

	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		return mockStreamQuadsBySubject(tc, subjectIRI)
	})

	svc := service.NewActivityService(mockStorage, mockParser, portmock.NewMediaStoragePortMock(mc), createTestFetcher(mc), service.ActivityServiceConfig{})

	task := model.InboundTask{
		ID:          "018c0000-0000-7000-8000-000000000001",
		ActivityIRI: "https://remote.com/act/add-1",
		ObjectIRI:   tc.collectionIRI,
		Payload:     []byte(`{"type":"Add","actor":"` + tc.actorIRI + `","target":"` + tc.collectionIRI + `","object":"https://remote.com/note/1"}`),
	}

	err := svc.ProcessInboundTask(ctx, task)
	if tc.expectError {
		if err == nil {
			t.Fatalf("Expected error containing %q, got nil", tc.errContains)
		}
		if !strings.Contains(err.Error(), tc.errContains) {
			t.Errorf("Expected error containing %q, got: %v", tc.errContains, err)
		}
	} else {
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
	}
}

func TestProcessInboundTask_AddRemove_FEP400e(t *testing.T) {
	tests := getAddRemoveTestCases()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runAddRemoveTestCase(t, tc)
		})
	}
}
