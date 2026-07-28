-- +goose Up
-- 1. Add hard threshold tracking boundaries to the multi-tenant scope
ALTER TABLE server_tenants ADD COLUMN IF NOT EXISTS storage_ceiling_bytes BIGINT NOT NULL DEFAULT 1073741824; -- Default to 1GB per tenant

-- 2. FIX: Alter the existing table structure to inject missing loop tracking columns safely
ALTER TABLE actor_media_ownership ADD COLUMN IF NOT EXISTS object_name TEXT NOT NULL UNIQUE;
ALTER TABLE actor_media_ownership ADD COLUMN IF NOT EXISTS file_size BIGINT NOT NULL DEFAULT 0;

-- 3. Generate strategic indexing matrices to protect high-concurrency window aggregations
CREATE INDEX IF NOT EXISTS idx_media_ownership_tenant_actor ON actor_media_ownership(tenant_id, actor_iri);

-- +goose Down
-- 1. Clean up indexes from memory
DROP INDEX IF EXISTS idx_media_ownership_tenant_actor;

-- 2. Prune the added tracking attributes to reverse the migration cleanly
ALTER TABLE actor_media_ownership DROP COLUMN IF EXISTS file_size;
ALTER TABLE actor_media_ownership DROP COLUMN IF EXISTS object_name;

-- 3. Remove the quota column extension from the tenant configuration table
ALTER TABLE server_tenants DROP COLUMN IF EXISTS storage_ceiling_bytes;
