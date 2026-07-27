-- +goose Up
-- Upgrade database schema to enforce structural dual-key storage boundaries.
ALTER TABLE local_actor_credentials ADD COLUMN private_key_ed25519_pem TEXT;
ALTER TABLE local_actor_credentials RENAME COLUMN private_key_pem TO private_key_rsa_pem;

-- +goose Down
ALTER TABLE local_actor_credentials RENAME COLUMN private_key_rsa_pem TO private_key_pem;
ALTER TABLE local_actor_credentials DROP COLUMN IF EXISTS private_key_ed25519_pem;
