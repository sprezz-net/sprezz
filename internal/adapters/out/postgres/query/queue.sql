-- name: IsDomainBlocked :one
SELECT EXISTS(
    SELECT 1 FROM blocked_domains WHERE domain_name = $1
) AS blocked;

-- name: InsertTenant :exec
INSERT INTO server_tenants (domain_name)
VALUES ($1)
ON CONFLICT (domain_name) DO NOTHING;

-- name: GetTenantID :one
SELECT id FROM server_tenants WHERE domain_name = $1;

-- name: EnqueueInboundActivity :exec
INSERT INTO inbound_activity_queue (id, activity_iri, object_iri, payload, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'pending', NOW(), NOW())
ON CONFLICT (activity_iri) DO NOTHING;

-- name: RecordTenantDelivery :exec
INSERT INTO activity_tenant_deliveries (activity_iri, tenant_id)
VALUES ($1, $2)
ON CONFLICT (activity_iri, tenant_id) DO NOTHING;

-- name: RecordActorInboxDelivery :exec
INSERT INTO actor_inbox_deliveries (actor_iri, activity_iri)
VALUES ($1, $2)
ON CONFLICT (actor_iri, activity_iri) DO NOTHING;

-- name: ClaimInboundTasks :many
SELECT id, activity_iri, object_iri, payload
FROM inbound_activity_queue
WHERE status = 'pending' OR status = 'failed'
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkInboundProcessing :exec
UPDATE inbound_activity_queue
SET status = 'processing', attempts = attempts + 1, updated_at = NOW()
WHERE id = ANY($1::uuid[]);

-- name: MarkInboundComplete :exec
UPDATE inbound_activity_queue
SET status = 'completed', updated_at = NOW()
WHERE id = $1;

-- name: MarkInboundFailed :exec
UPDATE inbound_activity_queue
SET status = 'failed', error_message = $2, updated_at = NOW()
WHERE id = $1;