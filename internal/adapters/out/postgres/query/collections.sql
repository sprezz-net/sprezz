-- name: GetInboxPayloads :many
SELECT q.payload
FROM actor_inbox_deliveries d
JOIN inbound_activity_queue q ON q.activity_iri = d.activity_iri
WHERE d.actor_iri = $1
ORDER BY d.created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetOutboxPayloads :many
SELECT payload
FROM outbound_activity_queue
WHERE actor_iri = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;