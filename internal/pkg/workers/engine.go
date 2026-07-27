package workers

import (
	"context"
	"sync"
	"time"
)

type Config struct {
	NumWorkers int
	BatchSize  int
	PollDelay  time.Duration
}

// BatchEngine is a generic, thread-safe background orchestration engine
// that claims tasks in batches and processes them concurrently via a worker pool.
type BatchEngine[T any] struct {
	cfg        Config
	claimBatch func(ctx context.Context, batchSize int) ([]T, error)
	runTask    func(ctx context.Context, task T)
}

// NewBatchEngine instantiates a generic loop framework on top of injectable behavioral closures.
func NewBatchEngine[T any](
	cfg Config,
	claimBatch func(ctx context.Context, batchSize int) ([]T, error),
	runTask func(ctx context.Context, task T),
) *BatchEngine[T] {
	if cfg.NumWorkers <= 0 {
		cfg.NumWorkers = 4
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.PollDelay <= 0 {
		cfg.PollDelay = 500 * time.Millisecond
	}
	return &BatchEngine[T]{
		cfg:        cfg,
		claimBatch: claimBatch,
		runTask:    runTask,
	}
}

// Start launches the background polling orchestrators and concurrent worker loops symmetrically.
func (e *BatchEngine[T]) Start(ctx context.Context) error {
	taskChan := make(chan T, e.cfg.BatchSize)
	var wg sync.WaitGroup

	// 1. Launch concurrent task-execution processing threads
	for i := 0; i < e.cfg.NumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskChan {
				e.runTask(ctx, task)
			}
		}()
	}

	// 2. Launch single background polling queue orchestrator thread
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(taskChan)
		ticker := time.NewTicker(e.cfg.PollDelay)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tasks, err := e.claimBatch(ctx, e.cfg.BatchSize)
				if err != nil || len(tasks) == 0 {
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
	}()

	wg.Wait()
	return nil
}
