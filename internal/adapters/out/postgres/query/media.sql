-- name: InsertMediaAttachment :one
INSERT INTO media_attachments (object_name, content_type, file_size)
VALUES ($1, $2, $3)
RETURNING id;

-- name: RegisterActorMediaOwnership :exec
INSERT INTO actor_media_ownership (actor_iri, tenant_id, media_attachment_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: LinkAttachmentToGraphVersion :exec
INSERT INTO rdf_graph_attachments (graph_id, media_attachment_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;
