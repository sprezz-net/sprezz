package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portstub"
	"sprezz/internal/domain/service"
)

type mockWorkerStorage struct {
	portstub.UnimplementedStoragePort // Composite fallback embedded stub (de-bloating layout)
	mu                                sync.Mutex
	tasks                             []model.InboundTask
	completedIDs                      map[string]struct{}
	failedTasks                       map[string]string
	privateKey                        string
}

func (m *mockWorkerStorage) ClaimInboundBatch(ctx context.Context, b int) ([]model.InboundTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := m.tasks
	m.tasks = nil
	return res, nil
}

func (m *mockWorkerStorage) MarkInboundComplete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completedIDs[id] = struct{}{}
	return nil
}

func (m *mockWorkerStorage) MarkInboundFailed(ctx context.Context, id, r string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedTasks[id] = r
	return nil
}

func (m *mockWorkerStorage) GetActorPrivateKey(ctx context.Context, a string) (string, error) {
	if m.privateKey == "error" {
		return "", errors.New("key resolved error")
	}
	return m.privateKey, nil
}

type mockActivityService struct {
	failID string
}

func (m *mockActivityService) ProcessInboundTask(ctx context.Context, t model.InboundTask) error {
	if t.ID == m.failID {
		return errors.New("simulated ingestion exception")
	}
	return nil
}
func (m *mockActivityService) DispatchOutboundActivity(ctx context.Context, a, ac string, p []byte) error {
	return nil
}
func (m *mockActivityService) GetFollowersTimeline(ctx context.Context, a string, l, o int) ([]string, error) {
	return nil, nil
}

type mockOutboundDispatcher struct {
	failInbox string
}

func (m *mockOutboundDispatcher) ForwardFederatedActivity(ctx context.Context, targetInbox, actorKeyID, privateKeyRSAPEM, privateKeyEd25519PEM string, payload []byte) error {
	if targetInbox == m.failInbox {
		return errors.New("network dispatch exception")
	}
	return nil
}

func TestWorkerEngines_UnifiedSymmetry(t *testing.T) {
	storage := &mockWorkerStorage{
		tasks: []model.InboundTask{
			{ID: "task-success-1", ActivityIRI: "https://remote.com", ObjectIRI: "https://sprezz.net"},
			{ID: "task-failure-2", ActivityIRI: "https://blocked.com", ObjectIRI: "https://sprezz.net"},
		},
		completedIDs: make(map[string]struct{}),
		failedTasks:  make(map[string]string),
		privateKey:   "-----BEGIN RSA PRIVATE KEY-----",
	}

	activitySvc := &mockActivityService{failID: "task-failure-2"}
	dispatcher := &mockOutboundDispatcher{failInbox: "https://blocked.com"}

	cfg := service.WorkerConfig{NumWorkers: 2, BatchSize: 5, PollDelay: 10 * time.Millisecond}
	outCfg := service.OutboundWorkerConfig{NumWorkers: 2, BatchSize: 5, PollDelay: 10 * time.Millisecond}

	inboundEngine := service.NewInboundWorkerEngine(cfg, storage, activitySvc)
	outboundEngine := service.NewOutboundWorkerEngine(outCfg, storage, dispatcher)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() { _ = inboundEngine.Start(ctx) }()
	go func() { _ = outboundEngine.Start(ctx) }()

	time.Sleep(30 * time.Millisecond)
	cancel()

	storage.mu.Lock()
	defer storage.mu.Unlock()

	if _, ok := storage.completedIDs["task-success-1"]; !ok {
		t.Error("Expected task-success-1 to be finalized and marked complete")
	}
	if _, ok := storage.failedTasks["task-failure-2"]; !ok {
		t.Error("Expected task-failure-2 to report error tracking telemetry")
	}
}
