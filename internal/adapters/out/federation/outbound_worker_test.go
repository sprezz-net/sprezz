package federation_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"sprezz/internal/adapters/out/federation"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/pkg/workers"
)

func TestOutboundWorkerEngine_Lifecycle(t *testing.T) {
	mc := minimock.NewController(t)

	var mu sync.Mutex
	tasks := []model.InboundTask{
		{ID: "outbound-success-1", ActivityIRI: "https://remote.com", ObjectIRI: "https://sprezz.net", Payload: []byte(`{}`)},
		{ID: "outbound-failure-2", ActivityIRI: "https://blocked.com", ObjectIRI: "https://sprezz.net", Payload: []byte(`{}`)},
	}
	completedIDs := make(map[string]struct{})
	failedTasks := make(map[string]string)

	storage := portmock.NewStoragePortMock(mc)
	storage.ClaimInboundBatchMock.Set(func(ctx context.Context, b int) ([]model.InboundTask, error) {
		mu.Lock()
		defer mu.Unlock()
		res := tasks
		tasks = nil
		return res, nil
	})
	storage.MarkInboundCompleteMock.Set(func(ctx context.Context, id string) error {
		mu.Lock()
		defer mu.Unlock()
		completedIDs[id] = struct{}{}
		return nil
	})
	storage.MarkInboundFailedMock.Set(func(ctx context.Context, id, r string) error {
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
