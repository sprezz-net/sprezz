package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports"
)

// WorkerConfig holds the shared performance configuration tuning attributes.
type WorkerConfig struct {
	NumWorkers int
	BatchSize  int
	PollDelay  time.Duration
}

// OutboundWorkerConfig maps directly to WorkerConfig to preserve main.go initialization APIs.
type OutboundWorkerConfig struct {
	NumWorkers int
	BatchSize  int
	PollDelay  time.Duration
}

// BatchWorkerEngine is a generic, thread-safe background orchestration engine
// that claims tasks in batches and processes them concurrently via a worker pool.
type BatchWorkerEngine[T any] struct {
	cfg        WorkerConfig
	claimBatch func(ctx context.Context, batchSize int) ([]T, error)
	runTask    func(ctx context.Context, task T)
}

// NewBatchWorkerEngine instantiates a generic loop framework on top of injectable behavioral closures.
func NewBatchWorkerEngine[T any](
	cfg WorkerConfig,
	claimBatch func(ctx context.Context, batchSize int) ([]T, error),
	runTask func(ctx context.Context, task T),
) *BatchWorkerEngine[T] {
	if cfg.NumWorkers <= 0 {
		cfg.NumWorkers = 4
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.PollDelay <= 0 {
		cfg.PollDelay = 500 * time.Millisecond
	}
	return &BatchWorkerEngine[T]{
		cfg:        cfg,
		claimBatch: claimBatch,
		runTask:    runTask,
	}
}

// Start launches the background polling orchestrators and concurrent worker loops symmetrically.
func (e *BatchWorkerEngine[T]) Start(ctx context.Context) error {
	taskChan := make(chan T, e.cfg.BatchSize)
	var wg sync.WaitGroup

	// 1. Launch concurrent task-execution processing threads
	for i := 0; i < e.cfg.NumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.workerLoop(ctx, taskChan)
		}()
	}

	// 2. Launch single background polling queue orchestrator thread
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(taskChan)
		e.orchestratorLoop(ctx, taskChan)
	}()

	wg.Wait()
	return nil
}

func (e *BatchWorkerEngine[T]) orchestratorLoop(ctx context.Context, taskChan chan<- T) {
	ticker := time.NewTicker(e.cfg.PollDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tasks, err := e.claimBatch(ctx, e.cfg.BatchSize)
			if err != nil {
				continue
			}

			if len(tasks) == 0 {
				continue
			}

			for _, task := range tasks {
				select {
				case <-ctx.Done():
					return
				case taskChan <- task:
				}
			}
		}
	}
}

func (e *BatchWorkerEngine[T]) workerLoop(ctx context.Context, taskChan <-chan T) {
	for task := range taskChan {
		e.runTask(ctx, task)
	}
}

// NewInboundWorkerEngine initializes the inbound ActivityPub ingestion engine using the generic framework.
func NewInboundWorkerEngine(cfg WorkerConfig, storage ports.StoragePort, svc ports.ActivityServicePort) *BatchWorkerEngine[model.InboundTask] {
	claimFn := func(ctx context.Context, batchSize int) ([]model.InboundTask, error) {
		return storage.ClaimInboundBatch(ctx, batchSize)
	}

	runFn := func(ctx context.Context, task model.InboundTask) {
		err := svc.ProcessInboundTask(ctx, task)
		if err != nil {
			reason := fmt.Sprintf("processing error: %v", err)
			_ = storage.MarkInboundFailed(ctx, task.ID, reason)
			return
		}
		_ = storage.MarkInboundComplete(ctx, task.ID)
	}

	return NewBatchWorkerEngine(cfg, claimFn, runFn)
}

// NewOutboundWorkerEngine initializes the federation distribution engine using the generic framework.
func NewOutboundWorkerEngine(cfg OutboundWorkerConfig, storage ports.StoragePort, dispatcher ports.OutboundDispatcher) *BatchWorkerEngine[model.InboundTask] {
	claimFn := func(ctx context.Context, batchSize int) ([]model.InboundTask, error) {
		return storage.ClaimInboundBatch(ctx, batchSize)
	}

	runFn := func(ctx context.Context, task model.InboundTask) {
		dualKeys, err := storage.GetActorDualKeys(ctx, task.ObjectIRI)
		if err != nil {
			_ = storage.MarkInboundFailed(ctx, task.ID, fmt.Sprintf("failed to resolve actor dual-key credentials: %v", err))
			return
		}

		keyID := task.ObjectIRI + "#main-key"

		// Use the PrivateKeyRSAPEM key from the dual-key record to maintain backward federation compatibility.
		err = dispatcher.ForwardFederatedActivity(ctx, task.ActivityIRI, keyID, dualKeys.PrivateKeyRSAPEM, task.Payload)
		if err != nil {
			reason := fmt.Sprintf("outbound transport dispatch failure: %v", err)
			_ = storage.MarkInboundFailed(ctx, task.ID, reason)
			return
		}

		_ = storage.MarkInboundComplete(ctx, task.ID)
	}

	return NewBatchWorkerEngine(WorkerConfig(cfg), claimFn, runFn)
}
