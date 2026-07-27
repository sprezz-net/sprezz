package federation

import (
	"context"
	"fmt"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/pkg/workers"
)

// NewOutboundWorkerEngine initializes the federation distribution engine using the generic framework.
func NewOutboundWorkerEngine(cfg workers.Config, storage port.StoragePort, dispatcher port.OutboundDispatcher) *workers.BatchEngine[model.InboundTask] {
	claimFn := func(ctx context.Context, batchSize int) ([]model.InboundTask, error) {
		return storage.ClaimInboundBatch(ctx, batchSize)
	}

	runFn := func(ctx context.Context, task model.InboundTask) {
		dualKeys, err := storage.GetActorDualKeys(ctx, task.ObjectIRI)
		if err != nil {
			_ = storage.MarkInboundFailed(ctx, task.ID, fmt.Sprintf("failed to resolve dual keys: %v", err))
			return
		}

		keyID := task.ObjectIRI + "#main-key"
		err = dispatcher.ForwardFederatedActivity(ctx, task.ActivityIRI, keyID, dualKeys.PrivateKeyRSAPEM, dualKeys.PrivateKeyEd25519PEM, task.Payload)
		if err != nil {
			_ = storage.MarkInboundFailed(ctx, task.ID, fmt.Sprintf("dispatch failure: %v", err))
			return
		}

		_ = storage.MarkInboundComplete(ctx, task.ID)
	}

	return workers.NewBatchEngine(cfg, claimFn, runFn)
}
