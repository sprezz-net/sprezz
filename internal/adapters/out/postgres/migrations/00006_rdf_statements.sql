-- +goose Up
-- Create the rdf_statements view to map universal quads to local multi-tenant partitions implicitly by domain
CREATE VIEW rdf_statements AS
SELECT
    d_sub.value AS subject,
    d_pred.value AS predicate,
    COALESCE(d_obj.value, q.literal_value)::TEXT AS object,
    st.id AS tenant_id
FROM rdf_quads q
JOIN rdf_dictionary d_sub ON q.subject_id = d_sub.id
JOIN rdf_dictionary d_pred ON q.predicate_id = d_pred.id
LEFT JOIN rdf_dictionary d_obj ON q.object_id = d_obj.id
JOIN server_tenants st ON lower(substring(d_sub.value from 'https?://([^/]+)')) = lower(st.domain_name);

-- +goose Down
DROP VIEW IF EXISTS rdf_statements;
