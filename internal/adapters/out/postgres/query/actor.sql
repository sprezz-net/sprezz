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
      ORDER BY inner_q.graph_id DESC
      LIMIT 1
  );

-- name: GetTenantDomainByID :one
SELECT domain_name
FROM server_tenants
WHERE id = $1;
