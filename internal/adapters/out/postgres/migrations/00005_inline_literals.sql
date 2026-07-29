-- +goose Up
-- 1. Drop the primary key constraint as object_id will become nullable
ALTER TABLE rdf_quads DROP CONSTRAINT IF EXISTS rdf_quads_pkey;

-- 2. Alter object_id to be nullable
ALTER TABLE rdf_quads ALTER COLUMN object_id DROP NOT NULL;

-- 3. Add literal_value column
ALTER TABLE rdf_quads ADD COLUMN literal_value TEXT;

-- 4. Add the check constraint to ensure only one of object_id or literal_value is populated
ALTER TABLE rdf_quads ADD CONSTRAINT chk_quad_object CHECK (
    (is_literal = TRUE AND literal_value IS NOT NULL AND object_id IS NULL) OR
    (is_literal = FALSE AND literal_value IS NULL AND object_id IS NOT NULL)
);

-- 5. Create unique constraints using indexes (PostgreSQL handles NULLs in UNIQUE INDEX partials gracefully)
CREATE UNIQUE INDEX idx_quads_non_literal_uniq ON rdf_quads (graph_id, subject_id, predicate_id, object_id) WHERE object_id IS NOT NULL;
CREATE UNIQUE INDEX idx_quads_literal_uniq ON rdf_quads (graph_id, subject_id, predicate_id, md5(literal_value)) WHERE object_id IS NULL;

-- +goose Down
-- 1. Drop unique indexes
DROP INDEX IF EXISTS idx_quads_literal_uniq;
DROP INDEX IF EXISTS idx_quads_non_literal_uniq;

-- 2. Drop the check constraint
ALTER TABLE rdf_quads DROP CONSTRAINT IF EXISTS chk_quad_object;

-- 3. Backfill or remove literal values if reversing (here we delete literal quads to safely restore NOT NULL constraint)
DELETE FROM rdf_quads WHERE object_id IS NULL;

-- 4. Drop literal_value column
ALTER TABLE rdf_quads DROP COLUMN IF EXISTS literal_value;

-- 5. Restore object_id to be NOT NULL
ALTER TABLE rdf_quads ALTER COLUMN object_id SET NOT NULL;

-- 6. Restore primary key constraint
ALTER TABLE rdf_quads ADD CONSTRAINT rdf_quads_pkey PRIMARY KEY (graph_id, subject_id, predicate_id, object_id);
