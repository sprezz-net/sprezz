// File: /internal/adapters/out/federation/outbound_worker_test.go
package federation_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"sprezz/internal/adapters/out/federation"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portstub"
	"sprezz/internal/pkg/workers"
)

type mockOutboundStorage struct {
	portstub.UnimplementedStoragePort
	mu           sync.Mutex
	tasks        []model.InboundTask
	completedIDs map[string]struct{}
	failedTasks  map[string]string
}

func (m *mockOutboundStorage) ClaimInboundBatch(ctx context.Context, b int) ([]model.InboundTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := m.tasks
	m.tasks = nil
	return res, nil
}

func (m *mockOutboundStorage) MarkInboundComplete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completedIDs[id] = struct{}{}
	return nil
}

func (m *mockOutboundStorage) MarkInboundFailed(ctx context.Context, id, r string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedTasks[id] = r
	return nil
}

func (m *mockOutboundStorage) GetActorDualKeys(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
	return &model.ActorDualKeys{
		PrivateKeyRSAPEM:     "-----BEGIN RSA PRIVATE KEY-----",
		PrivateKeyEd25519PEM: "-----BEGIN PRIVATE KEY-----",
	}, nil
}

type mockOutboundDispatcher struct {
	failInbox string
}

func (m *mockOutboundDispatcher) ForwardFederatedActivity(ctx context.Context, targetInbox, actorKeyID, rsaPEM, edPEM string, payload []byte) error {
	if targetInbox == m.failInbox {
		return errors.New("network dispatch exception")
	}
	return nil
}

func TestOutboundWorkerEngine_Lifecycle(t *testing.T) {
	storage := &mockOutboundStorage{
		tasks: []model.InboundTask{
			{ID: "outbound-success-1", ActivityIRI: "https://remote.com", ObjectIRI: "https://sprezz.net", Payload: []byte(`{}`)},
			{ID: "outbound-failure-2", ActivityIRI: "https://blocked.com", ObjectIRI: "https://sprezz.net", Payload: []byte(`{}`)},
		},
		completedIDs: make(map[string]struct{}),
		failedTasks:  make(map[string]string),
	}

	dispatcher := &mockOutboundDispatcher{failInbox: "https://blocked.com"}
	cfg := workers.Config{NumWorkers: 2, BatchSize: 5, PollDelay: 10 * time.Millisecond}

	engine := federation.NewOutboundWorkerEngine(cfg, storage, dispatcher)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() { _ = engine.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	storage.mu.Lock()
	defer storage.mu.Unlock()

	if _, ok := storage.completedIDs["outbound-success-1"]; !ok {
		t.Error("Expected outbound-success-1 to be finalized and marked complete")
	}
	if _, ok := storage.failedTasks["outbound-failure-2"]; !ok {
		t.Error("Expected outbound-failure-2 to report error tracking telemetry")
	}
}
