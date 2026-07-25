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
