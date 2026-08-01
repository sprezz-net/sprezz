package federation_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"sprezz/internal/adapters/out/federation"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/pkg/workers"

	"github.com/gojuno/minimock/v3"
)

func TestOutboundWorkerEngine_Lifecycle(t *testing.T) {
	mc := minimock.NewController(t)

	var mu sync.Mutex
	tasks := []model.OutboundTask{
		{ID: "outbound-success-1", ActivityIRI: "https://remote.com", ActorIRI: "https://sprezz.net", Payload: []byte(`{}`)},
		{ID: "outbound-failure-2", ActivityIRI: "https://blocked.com", ActorIRI: "https://sprezz.net", Payload: []byte(`{}`)},
	}
	completedIDs := make(map[string]struct{})
	failedTasks := make(map[string]string)

	storage := portmock.NewStoragePortMock(mc)
	storage.ClaimOutboundBatchMock.Set(func(ctx context.Context, b int) ([]model.OutboundTask, error) {
		mu.Lock()
		defer mu.Unlock()
		res := tasks
		tasks = nil
		return res, nil
	})
	storage.MarkOutboundCompleteMock.Set(func(ctx context.Context, id string) error {
		mu.Lock()
		defer mu.Unlock()
		completedIDs[id] = struct{}{}
		return nil
	})
	storage.MarkOutboundFailedMock.Set(func(ctx context.Context, id, r string) error {
		mu.Lock()
		defer mu.Unlock()
		failedTasks[id] = r
		return nil
	})
	storage.GetActorDualKeysMock.Set(func(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
		return &model.ActorDualKeys{
			PrivateKeyRSAPEM:     "-----BEGIN RSA PRIVATE KEY-----",
			PrivateKeyEd25519PEM: "-----BEGIN PRIVATE KEY-----",
		}, nil
	})

	dispatcher := portmock.NewOutboundDispatcherMock(mc)
	dispatcher.ForwardFederatedActivityMock.Set(func(ctx context.Context, targetInbox, actorKeyID, rsaPEM, edPEM string, payload []byte) error {
		if targetInbox == "https://blocked.com" {
			return errors.New("network dispatch exception")
		}
		return nil
	})

	cfg := workers.Config{NumWorkers: 2, BatchSize: 5, PollDelay: 10 * time.Millisecond}

	engine := federation.NewOutboundWorkerEngine(cfg, storage, dispatcher)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() { _ = engine.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if _, ok := completedIDs["outbound-success-1"]; !ok {
		t.Error("Expected outbound-success-1 to be finalized and marked complete")
	}
	if _, ok := failedTasks["outbound-failure-2"]; !ok {
		t.Error("Expected outbound-failure-2 to report error tracking telemetry")
	}
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{err: nil, expected: false},
		{err: errors.New("remote network endpoint refused activity delivery: status code 500"), expected: true},
		{err: errors.New("remote network endpoint refused activity delivery: status code 404"), expected: false},
		{err: errors.New("remote network endpoint refused activity delivery: status code 408"), expected: true},
		{err: errors.New("remote network endpoint refused activity delivery: status code 410"), expected: false},
		{err: errors.New("connection timeout"), expected: true},
	}

	for i, tc := range tests {
		res := federation.ExportIsTransientError(tc.err)
		if res != tc.expected {
			t.Errorf("test %d: expected %v, got %v", i, tc.expected, res)
		}
	}
}

func TestGetRetryDelay(t *testing.T) {
	tests := []struct {
		attempts int
		expected time.Duration
	}{
		{attempts: 0, expected: 1 * time.Second},
		{attempts: 1, expected: 2 * time.Second},
		{attempts: 2, expected: 4 * time.Second},
		{attempts: 5, expected: 32 * time.Second},
		{attempts: 20, expected: 2 * time.Hour}, // Max delay capped
	}

	for i, tc := range tests {
		res := federation.ExportGetRetryDelay(tc.attempts)
		if res != tc.expected {
			t.Errorf("test %d: expected %v, got %v", i, tc.expected, res)
		}
	}
}
