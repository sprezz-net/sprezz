-- +goose Up
-- Idempotency registry for processed inbound activities to prevent duplicate processing
CREATE TABLE processed_activities (
    activity_iri TEXT PRIMARY KEY,
    processed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- +goose Down
DROP TABLE processed_activities;
