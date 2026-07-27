package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"sprezz/internal/adapters/in/worker"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portstub"
	"sprezz/internal/pkg/workers"
)

type mockInboundStorage struct {
	portstub.UnimplementedStoragePort
	mu           sync.Mutex
	tasks        []model.InboundTask
	completedIDs map[string]struct{}
	failedTasks  map[string]string
}

func (m *mockInboundStorage) ClaimInboundBatch(ctx context.Context, b int) ([]model.InboundTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := m.tasks
	m.tasks = nil
	return res, nil
}

func (m *mockInboundStorage) MarkInboundComplete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completedIDs[id] = struct{}{}
	return nil
}

func (m *mockInboundStorage) MarkInboundFailed(ctx context.Context, id, r string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedTasks[id] = r
	return nil
}

type mockActivityService struct {
	portstub.UnimplementedActivityServicePort
	failID string
}

func (m *mockActivityService) ProcessInboundTask(ctx context.Context, t model.InboundTask) error {
	if t.ID == m.failID {
		return errors.New("simulated ingestion exception")
	}
	return nil
}

func TestInboundWorkerEngine_Lifecycle(t *testing.T) {
	storage := &mockInboundStorage{
		tasks: []model.InboundTask{
			{ID: "inbound-success-1", ActivityIRI: "https://remote.com", ObjectIRI: "https://sprezz.net"},
			{ID: "inbound-failure-2", ActivityIRI: "https://blocked.com", ObjectIRI: "https://sprezz.net"},
		},
		completedIDs: make(map[string]struct{}),
		failedTasks:  make(map[string]string),
	}

	activitySvc := &mockActivityService{failID: "inbound-failure-2"}
	cfg := workers.Config{NumWorkers: 2, BatchSize: 5, PollDelay: 10 * time.Millisecond}

	engine := worker.NewInboundWorkerEngine(cfg, storage, activitySvc)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() { _ = engine.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	storage.mu.Lock()
	defer storage.mu.Unlock()

	if _, ok := storage.completedIDs["inbound-success-1"]; !ok {
		t.Error("Expected inbound-success-1 to be finalized and marked complete")
	}
	if _, ok := storage.failedTasks["inbound-failure-2"]; !ok {
		t.Error("Expected inbound-failure-2 to report error tracking telemetry")
	}
}
