-- +goose Up
ALTER TABLE server_tenants ADD COLUMN tenant_uuid UUID UNIQUE NOT NULL DEFAULT gen_random_uuid();

-- +goose Down
ALTER TABLE server_tenants DROP COLUMN tenant_uuid;
