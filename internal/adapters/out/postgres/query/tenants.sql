-- name: IsDomainBlocked :one
SELECT EXISTS(
    SELECT 1 FROM blocked_domains WHERE domain_name = $1
) AS blocked;

-- name: GetTenantByDomain :one
SELECT id, domain_name FROM server_tenants WHERE domain_name = $1;

-- name: InsertTenant :one
INSERT INTO server_tenants (domain_name)
VALUES ($1)
ON CONFLICT (domain_name) DO UPDATE SET domain_name = EXCLUDED.domain_name
RETURNING id, domain_name;

-- name: GetActorCredentialsByUsername :one
SELECT actor_iri, tenant_id, username, private_key_pem
FROM local_actor_credentials
WHERE tenant_id = $1 AND username = $2;

-- name: InsertActorCredentials :exec
INSERT INTO local_actor_credentials (actor_iri, identity_guid, tenant_id, username, private_key_pem)
VALUES ($1, NULL, $2, $3, $4)
ON CONFLICT (tenant_id, username) DO NOTHING;
