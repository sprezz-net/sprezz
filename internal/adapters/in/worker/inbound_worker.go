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
		if err := svc.ProcessInboundTask(ctx, task); err != nil {
			_ = storage.MarkInboundFailed(ctx, task.ID, fmt.Sprintf("processing error: %v", err))
			return
		}
		_ = storage.MarkInboundComplete(ctx, task.ID)
	}

	return workers.NewBatchEngine(cfg, claimFn, runFn)
}
