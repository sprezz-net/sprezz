package model

import (
	"context"
	"testing"
)

func TestDetach(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, TenantIDKey, int32(42))

	detached := Detach(ctx)

	// Cancel the parent context
	cancel()

	// Verify the detached context is NOT cancelled
	if err := detached.Err(); err != nil {
		t.Errorf("expected detached context to have no error, got: %v", err)
	}

	select {
	case <-detached.Done():
		t.Error("expected detached context Done channel to not be closed")
	default:
	}

	// Verify the detached context has no deadline
	deadline, ok := detached.Deadline()
	if ok || !deadline.IsZero() {
		t.Errorf("expected no deadline on detached context, got: %v", deadline)
	}

	// Verify value retrieval works perfectly
	val := detached.Value(TenantIDKey)
	if tenantID, ok := val.(int32); !ok || tenantID != 42 {
		t.Errorf("expected value for TenantIDKey to be 42, got: %v", val)
	}
}
