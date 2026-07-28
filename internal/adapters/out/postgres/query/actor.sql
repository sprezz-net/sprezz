-- name: GetActorQuadsByUsername :many
SELECT
    q.graph_id,
    s_term.value AS subject,
    p_term.value AS predicate,
    o_term.value AS object
FROM rdf_quads q
JOIN rdf_graphs g ON q.graph_id = g.id
JOIN rdf_dictionary s_term ON q.subject_id = s_term.id
JOIN rdf_dictionary p_term ON q.predicate_id = p_term.id
JOIN rdf_dictionary o_term ON q.object_id = o_term.id
WHERE q.graph_id = (
      -- Isolate the latest known graph version associated with this actor handle
      SELECT inner_q.graph_id
      FROM rdf_quads inner_q
      JOIN rdf_graphs inner_g ON inner_q.graph_id = inner_g.id
      JOIN rdf_dictionary inner_s ON inner_q.subject_id = inner_s.id
      JOIN rdf_dictionary inner_p ON inner_q.predicate_id = inner_p.id
      JOIN rdf_dictionary inner_o ON inner_q.object_id = inner_o.id
      WHERE inner_p.value = 'https://www.w3.org/ns/activitystreams#preferredUsername'
        -- Explicit parameter type overrides for clean sqlc struct generation:
        AND inner_o.value = @username::text
        AND inner_s.value LIKE @tenant_prefix::text
        -- DATABASE-LEVEL OPTIMIZATION: Ensure the subject is semantically an Actor Type
        AND EXISTS (
            SELECT 1
            FROM rdf_quads type_q
            JOIN rdf_dictionary type_p ON type_q.predicate_id = type_p.id
            JOIN rdf_dictionary type_o ON type_q.object_id = type_o.id
            WHERE type_q.graph_id = inner_q.graph_id
              AND type_q.subject_id = inner_q.subject_id
              AND type_p.value = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#type'
              AND type_o.value IN (
                  'https://www.w3.org/ns/activitystreams#Person',
                  'https://www.w3.org/ns/activitystreams#Service',
                  'https://www.w3.org/ns/activitystreams#Group',
                  'https://www.w3.org/ns/activitystreams#Organization',
                  'https://www.w3.org/ns/activitystreams#Application'
              )
        )
      ORDER BY inner_q.graph_id DESC
      LIMIT 1
  );

-- name: GetActorQuadsByIRI :many
SELECT
    q.graph_id,
    s_term.value AS subject,
    p_term.value AS predicate,
    o_term.value AS object
FROM rdf_quads q
JOIN rdf_graphs g ON q.graph_id = g.id
JOIN rdf_dictionary s_term ON q.subject_id = s_term.id
JOIN rdf_dictionary p_term ON q.predicate_id = p_term.id
JOIN rdf_dictionary o_term ON q.object_id = o_term.id
WHERE q.graph_id = (
      -- Instantly isolate the latest historical graph version block explicitly matching the actor's stable ID
      SELECT inner_q.graph_id
      FROM rdf_quads inner_q
      JOIN rdf_graphs inner_g ON inner_q.graph_id = inner_g.id
      JOIN rdf_dictionary inner_s ON inner_q.subject_id = inner_s.id
      WHERE inner_s.value = $1
        -- DATABASE-LEVEL OPTIMIZATION: Ensure the subject is semantically an Actor Type
        AND EXISTS (
            SELECT 1
            FROM rdf_quads type_q
            JOIN rdf_dictionary type_p ON type_q.predicate_id = type_p.id
            JOIN rdf_dictionary type_o ON type_q.object_id = type_o.id
            WHERE type_q.graph_id = inner_q.graph_id
              AND type_q.subject_id = inner_q.subject_id
              AND type_p.value = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#type'
              AND type_o.value IN (
                  'https://www.w3.org/ns/activitystreams#Person',
                  'https://www.w3.org/ns/activitystreams#Service',
                  'https://www.w3.org/ns/activitystreams#Group',
                  'https://www.w3.org/ns/activitystreams#Organization',
                  'https://www.w3.org/ns/activitystreams#Application'
              )
        )
      ORDER BY inner_q.graph_id DESC
      LIMIT 1
  );

-- name: GetTenantDomainByID :one
SELECT domain_name
FROM server_tenants
WHERE id = $1;

-- name: GetActorIRIByAlias :one
SELECT s_term.value AS subject
FROM rdf_quads q
JOIN rdf_dictionary s_term ON q.subject_id = s_term.id
JOIN rdf_dictionary p_term ON q.predicate_id = p_term.id
JOIN rdf_dictionary o_term ON q.object_id = o_term.id
WHERE p_term.value = 'https://www.w3.org/ns/activitystreams#alsoKnownAs'
  AND o_term.value = $1
LIMIT 1;
