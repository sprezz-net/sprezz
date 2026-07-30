package worker

import (
	"context"
	"fmt"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/pkg/workers"
)

// NewInboundWorkerEngine initializes the inbound ActivityPub ingestion engine using the generic framework.
func NewInboundWorkerEngine(cfg workers.Config, storage port.StoragePort, svc port.ActivityServicePort) *workers.BatchEngine[model.InboundTask] {
	claimFn := func(ctx context.Context, batchSize int) ([]model.InboundTask, error) {
		return storage.ClaimInboundBatch(ctx, batchSize)
	}

	runFn := func(ctx context.Context, task model.InboundTask) {
		// Attempt to record the activity as processed to prevent duplicate executions (Idempotency check)
		recorded, err := storage.RecordProcessedActivity(ctx, task.ActivityIRI)
		if err != nil {
			_ = storage.MarkInboundFailed(ctx, task.ID, fmt.Sprintf("idempotency check failure: %v", err))
			return
		}
		if !recorded {
			// This activity was already processed successfully or is currently being processed.
			// Mark it complete in the queue and skip domain services to prevent duplicates!
			_ = storage.MarkInboundComplete(ctx, task.ID)
			return
		}

		if err := svc.ProcessInboundTask(ctx, task); err != nil {
			_ = storage.MarkInboundFailed(ctx, task.ID, fmt.Sprintf("processing error: %v", err))
			return
		}
		_ = storage.MarkInboundComplete(ctx, task.ID)
	}

	return workers.NewBatchEngine(cfg, claimFn, runFn)
}
