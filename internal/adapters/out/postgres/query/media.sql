-- name: InsertMediaAttachment :one
INSERT INTO media_attachments (object_name, original_name, sha256_hex, content_type, file_size)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (object_name) DO UPDATE
SET sha256_hex = EXCLUDED.sha256_hex -- Ensure fallback idempotency
RETURNING id;

-- name: RegisterActorMediaOwnership :exec
INSERT INTO actor_media_ownership (actor_iri, tenant_id, media_attachment_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: LinkAttachmentToGraphVersion :exec
INSERT INTO rdf_graph_attachments (graph_id, media_attachment_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: GetTenantStorageUsageAndCeiling :one
SELECT
    st.storage_ceiling_bytes,
    COALESCE(SUM(am.file_size), 0)::BIGINT as current_usage_bytes
FROM server_tenants st
LEFT JOIN actor_media_ownership am ON am.tenant_id = st.id
WHERE st.id = $1
GROUP BY st.id, st.storage_ceiling_bytes;

-- name: RecordMediaAttachment :exec
INSERT INTO actor_media_ownership (
    tenant_id,
    actor_iri,
    object_name,
    file_size
) VALUES (
    $1, $2, $3, $4
);

-- name: RemoveMediaAttachment :exec
DELETE FROM actor_media_ownership
WHERE object_name = $1; -- Keep this as object_name
