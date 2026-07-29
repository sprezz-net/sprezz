package federation

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/pkg/workers"
)

// NewOutboundWorkerEngine initializes the federation distribution engine using the generic framework.
func NewOutboundWorkerEngine(cfg workers.Config, storage port.StoragePort, dispatcher port.OutboundDispatcher) *workers.BatchEngine[model.OutboundTask] {
	claimFn := func(ctx context.Context, batchSize int) ([]model.OutboundTask, error) {
		return storage.ClaimOutboundBatch(ctx, batchSize)
	}

	runFn := func(ctx context.Context, task model.OutboundTask) {
		dualKeys, err := storage.GetActorDualKeys(ctx, task.ActorIRI)
		if err != nil {
			_ = storage.MarkOutboundFailed(ctx, task.ID, fmt.Sprintf("failed to resolve dual keys: %v", err))
			return
		}

		keyID := task.ActorIRI + "#main-key"
		err = dispatcher.ForwardFederatedActivity(ctx, task.ActivityIRI, keyID, dualKeys.PrivateKeyRSAPEM, dualKeys.PrivateKeyEd25519PEM, task.Payload)
		if err != nil {
			// Determine if the dispatch failure is transient (and eligible for backoff) or permanent.
			if isTransientError(err) {
				// Mark as failed so it can be claimed again for retry in the next processing run,
				// or let it retry naturally.
				_ = storage.MarkOutboundFailed(ctx, task.ID, fmt.Sprintf("transient dispatch failure: %v", err))
			} else {
				// Permanent failure (e.g. 400, 401, 403, 404, 410) - we mark as failed and do not retry
				_ = storage.MarkOutboundFailed(ctx, task.ID, fmt.Sprintf("permanent dispatch failure: %v", err))
			}
			return
		}

		_ = storage.MarkOutboundComplete(ctx, task.ID)
	}

	return workers.NewBatchEngine(cfg, claimFn, runFn)
}

// isTransientError parses HTTP status and connection error patterns to identify retryable transient conditions.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "status code") {
		parts := strings.Split(msg, "status code ")
		if len(parts) > 1 {
			codeStr := strings.TrimSpace(parts[1])
			if code, convErr := strconv.Atoi(codeStr); convErr == nil {
				// Transient status codes (e.g. 408 Timeout, 429 Too Many Requests, or 5xx Server Errors)
				if code == 408 || code == 429 || code >= 500 {
					return true
				}
				return false
			}
		}
	}
	// Connection issues, context timeouts, and name resolution failures are treated as transient
	return true
}

// GetRetryDelay computes the truncated exponential backoff delay based on the attempt count.
func GetRetryDelay(attempts int) time.Duration {
	baseDelay := 1 * time.Second
	maxDelay := 2 * time.Hour

	if attempts <= 0 {
		return baseDelay
	}

	// Exponential backoff scaling
	tempDelay := baseDelay * (1 << uint(attempts))
	if tempDelay > maxDelay || tempDelay < baseDelay {
		return maxDelay
	}
	return tempDelay
}
