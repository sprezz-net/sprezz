-- name: CreateGraphVersion :one
INSERT INTO rdf_graphs (activity_id, object_iri, payload, created_at)
VALUES ($1, $2, $3, NOW())
RETURNING id;

-- name: GetDictionaryID :one
SELECT id FROM rdf_dictionary WHERE value = $1;

-- name: InsertDictionaryValue :one
WITH ins AS (
    INSERT INTO rdf_dictionary (value)
    VALUES ($1)
    ON CONFLICT (value) DO NOTHING
    RETURNING id
)
SELECT id FROM ins
UNION ALL
SELECT id FROM rdf_dictionary WHERE value = $1
LIMIT 1;

-- name: InsertQuad :exec
INSERT INTO rdf_quads (graph_id, subject_id, predicate_id, object_id, is_literal, literal_value)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT DO NOTHING;

-- name: RemoveQuadEdge :exec
DELETE FROM rdf_quads
WHERE subject_id = $1 AND predicate_id = $2 AND (object_id = $3 OR literal_value = $4);

-- name: GetLatestPayload :one
SELECT payload
FROM rdf_graphs
WHERE object_iri = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: GetSubjectQuads :many
SELECT q.graph_id, d_pred.value AS predicate, COALESCE(d_obj.value, q.literal_value)::TEXT AS object, q.is_literal
FROM rdf_quads q
JOIN rdf_dictionary d_pred ON q.predicate_id = d_pred.id
LEFT JOIN rdf_dictionary d_obj ON q.object_id = d_obj.id
WHERE q.subject_id = $1;

-- name: GetStatementsBySubjectIsolated :many
SELECT predicate, object
FROM rdf_statements
WHERE subject = $1 AND tenant_id = $2;
