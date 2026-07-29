-- +goose Up
-- Add retry schedule and telemetry columns to outbound_activity_queue
ALTER TABLE outbound_activity_queue
ADD COLUMN next_run_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
ADD COLUMN error_message TEXT;

-- Index for optimized claim filtering under heavy workloads
CREATE INDEX idx_outbound_queue_process ON outbound_activity_queue(status, next_run_at)
WHERE status = 'pending' OR status = 'failed';

-- +goose Down
DROP INDEX IF EXISTS idx_outbound_queue_process;

ALTER TABLE outbound_activity_queue
DROP COLUMN IF EXISTS next_run_at,
DROP COLUMN IF EXISTS error_message;
