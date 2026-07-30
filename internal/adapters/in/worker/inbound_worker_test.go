package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"sprezz/internal/adapters/in/worker"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/pkg/workers"
)

func TestInboundWorkerEngine_Lifecycle(t *testing.T) {
	mc := minimock.NewController(t)

	var mu sync.Mutex
	tasks := []model.InboundTask{
		{ID: "inbound-success-1", ActivityIRI: "https://remote.com", ObjectIRI: "https://sprezz.net"},
		{ID: "inbound-failure-2", ActivityIRI: "https://blocked.com", ObjectIRI: "https://sprezz.net"},
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
	storage.RecordProcessedActivityMock.Set(func(ctx context.Context, activityIRI string) (bool, error) {
		return true, nil
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

	activitySvc := portmock.NewActivityServicePortMock(mc)
	activitySvc.ProcessInboundTaskMock.Set(func(ctx context.Context, t model.InboundTask) error {
		if t.ID == "inbound-failure-2" {
			return errors.New("simulated ingestion exception")
		}
		return nil
	})

	cfg := workers.Config{NumWorkers: 2, BatchSize: 5, PollDelay: 10 * time.Millisecond}

	engine := worker.NewInboundWorkerEngine(cfg, storage, activitySvc)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() { _ = engine.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if _, ok := completedIDs["inbound-success-1"]; !ok {
		t.Error("Expected inbound-success-1 to be finalized and marked complete")
	}
	if _, ok := failedTasks["inbound-failure-2"]; !ok {
		t.Error("Expected inbound-failure-2 to report error tracking telemetry")
	}
}

func TestInboundWorkerEngine_Idempotency(t *testing.T) {
	mc := minimock.NewController(t)

	var mu sync.Mutex
	tasks := []model.InboundTask{
		{ID: "inbound-dup-1", ActivityIRI: "https://remote.com/retry", ObjectIRI: "https://sprezz.net"},
	}
	completedIDs := make(map[string]struct{})

	storage := portmock.NewStoragePortMock(mc)
	storage.ClaimInboundBatchMock.Set(func(ctx context.Context, b int) ([]model.InboundTask, error) {
		mu.Lock()
		defer mu.Unlock()
		res := tasks
		tasks = nil
		return res, nil
	})
	// Simulate that this activity has ALREADY been processed elsewhere/previously
	storage.RecordProcessedActivityMock.Set(func(ctx context.Context, activityIRI string) (bool, error) {
		if activityIRI == "https://remote.com/retry" {
			return false, nil
		}
		return true, nil
	})
	storage.MarkInboundCompleteMock.Set(func(ctx context.Context, id string) error {
		mu.Lock()
		defer mu.Unlock()
		completedIDs[id] = struct{}{}
		return nil
	})

	activitySvc := portmock.NewActivityServicePortMock(mc)

	cfg := workers.Config{NumWorkers: 1, BatchSize: 5, PollDelay: 10 * time.Millisecond}

	engine := worker.NewInboundWorkerEngine(cfg, storage, activitySvc)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() { _ = engine.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if _, ok := completedIDs["inbound-dup-1"]; !ok {
		t.Error("Expected duplicate task to be marked complete directly")
	}
}
