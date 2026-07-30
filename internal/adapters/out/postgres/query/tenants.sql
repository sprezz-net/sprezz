-- name: IsDomainBlocked :one
SELECT EXISTS(
    SELECT 1 FROM blocked_domains WHERE domain_name = $1
) AS blocked;

-- name: GetTenantByDomain :one
SELECT id, domain_name FROM server_tenants WHERE domain_name = $1;

-- name: InsertTenant :one
INSERT INTO server_tenants (tenant_uuid, domain_name)
VALUES ($1, $2)
ON CONFLICT (domain_name) DO UPDATE SET tenant_uuid = EXCLUDED.tenant_uuid
RETURNING id, tenant_uuid, domain_name;

-- name: GetAllTenants :many
SELECT id, tenant_uuid, domain_name FROM server_tenants;

-- name: GetActorCredentialsByUsername :one
SELECT actor_iri, tenant_id, username, private_key_rsa_pem, private_key_ed25519_pem
FROM local_actor_credentials
WHERE tenant_id = $1 AND username = $2;

-- name: InsertActorCredentials :exec
INSERT INTO local_actor_credentials (actor_iri, identity_guid, tenant_id, username, private_key_rsa_pem, private_key_ed25519_pem)
VALUES ($1, NULL, $2, $3, $4, $5)
ON CONFLICT (tenant_id, username) DO NOTHING;
