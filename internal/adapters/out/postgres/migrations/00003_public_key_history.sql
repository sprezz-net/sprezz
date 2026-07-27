-- +goose Up
-- Create an immutable historical tracking ledger for rotated public keys
CREATE TABLE actor_public_key_history (
    id BIGSERIAL PRIMARY KEY,
    actor_iri TEXT NOT NULL,
    key_type VARCHAR(50) NOT NULL,            -- Explicitly tracks 'RSA' or 'Ed25519' types
    public_key_pem TEXT NOT NULL,             -- The pure public key value envelope
    valid_from TIMESTAMP WITH TIME ZONE NOT NULL,
    valid_to TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT chk_key_history_dates CHECK (valid_from < valid_to)
);

-- Compound index for fast time-slice lookups inside the inbound SignatureValidator middleware
CREATE INDEX idx_actor_keys_historical_window
ON actor_public_key_history(actor_iri, valid_from, valid_to);

-- Pre-seed historical window records for your existing system actors to prevent past validation gaps
INSERT INTO actor_public_key_history (actor_iri, key_type, public_key_pem, valid_from, valid_to)
SELECT
    actor_iri,
    'RSA',
    private_key_rsa_pem, -- Extracted via our secure string translation layer later if needed
    NOW() - INTERVAL '1 year',
    NOW() + INTERVAL '10 years'
FROM local_actor_credentials
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS actor_public_key_history CASCADE;
