CREATE TYPE activity_status AS ENUM ('pending', 'processing', 'completed', 'failed');

-- Domain Multi-Tenancy Management
CREATE TABLE server_tenants (
    id SERIAL PRIMARY KEY,
    tenant_uuid UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    domain_name TEXT UNIQUE NOT NULL,
    storage_ceiling_bytes BIGINT NOT NULL DEFAULT 1073741824 -- Default to 1GB per tenant
);

-- Federation Blocklist (Defederation Early-Exit)
CREATE TABLE blocked_domains (
    domain_name TEXT PRIMARY KEY,
    blocked_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Nomadic Identity Registry (Zot6 / Nomad Extension)
CREATE TABLE nomadic_identities (
    guid TEXT PRIMARY KEY,                       -- Immutable global unique identifier
    primary_hub_url TEXT NOT NULL,               -- Current active routing location
    master_public_key_pem TEXT NOT NULL,         -- Root identity cryptographic key
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Tracking physical instance clones of a nomadic user profile
CREATE TABLE identity_clones (
    id BIGSERIAL PRIMARY KEY,
    identity_guid TEXT NOT NULL REFERENCES nomadic_identities(guid) ON DELETE CASCADE,
    hub_url TEXT NOT NULL,                       -- Physical URL where this clone profile lives
    is_local BOOLEAN DEFAULT FALSE,              -- True if this database instance hosts this exact clone
    synchronized_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE (identity_guid, hub_url)
);
CREATE INDEX idx_clone_lookup ON identity_clones(identity_guid);

-- Local Identity Registry & Outbound Credentials
CREATE TABLE local_actor_credentials (
    actor_iri TEXT PRIMARY KEY,                  -- Public-facing ActivityPub URI
    identity_guid TEXT REFERENCES nomadic_identities(guid) ON DELETE SET NULL,
    tenant_id INT NOT NULL REFERENCES server_tenants(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    private_key_rsa_pem TEXT NOT NULL,
    private_key_ed25519_pem TEXT,
    UNIQUE (tenant_id, username)
);

-- Create an immutable historical tracking ledger for rotated public keys
CREATE TABLE actor_public_key_history (
    id BIGSERIAL PRIMARY KEY,
    actor_iri TEXT NOT NULL,
    key_type VARCHAR(50) NOT NULL,            -- Explicitly tracks 'RSA' or 'Ed25519' types
    public_key_pem TEXT NOT NULL,             -- The pure public key value envelope
    valid_from TIMESTAMP WITH TIME ZONE NOT NULL,
    valid_to TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT chk_key_history_dates CHECK (valid_from < valid_to)
);
-- Compound index for fast time-slice lookups inside the inbound SignatureValidator middleware
CREATE INDEX idx_actor_keys_historical_window ON actor_public_key_history(actor_iri, valid_from, valid_to);

-- Inbound Lockless Buffering Cache (Deduplicated)
CREATE TABLE inbound_activity_queue (
    id UUID PRIMARY KEY,                         -- Time-ordered UUIDv7 in Go
    activity_iri TEXT UNIQUE NOT NULL,
    object_iri TEXT NOT NULL,
    payload JSONB NOT NULL,
    status activity_status DEFAULT 'pending',
    attempts INT DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_inbound_queue_process ON inbound_activity_queue(status, updated_at) WHERE status = 'pending' OR status = 'failed';

-- Idempotency registry for processed inbound activities to prevent duplicate processing
CREATE TABLE processed_activities (
    activity_iri TEXT PRIMARY KEY,
    processed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Multi-Tenant Router Cross-References
CREATE TABLE activity_tenant_deliveries (
    id BIGSERIAL PRIMARY KEY,
    activity_iri TEXT NOT NULL,
    tenant_id INT NOT NULL REFERENCES server_tenants(id) ON DELETE CASCADE,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE (activity_iri, tenant_id)
);
CREATE INDEX idx_delivery_tenant ON activity_tenant_deliveries(tenant_id);

-- Explicit Inbox Actor Deliveries (OrderedCollection Backend)
CREATE TABLE actor_inbox_deliveries (
    id BIGSERIAL PRIMARY KEY,
    actor_iri TEXT NOT NULL,
    activity_iri TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE (actor_iri, activity_iri)
);
CREATE INDEX idx_actor_inbox_chronological ON actor_inbox_deliveries(actor_iri, created_at DESC);

-- Outbound Async Federation Message Queue
CREATE TABLE outbound_activity_queue (
    id UUID PRIMARY KEY,                         -- UUIDv7
    activity_iri TEXT UNIQUE NOT NULL,
    actor_iri TEXT NOT NULL REFERENCES local_actor_credentials(actor_iri) ON DELETE CASCADE,
    payload JSONB NOT NULL,
    status activity_status DEFAULT 'pending',
    attempts INT DEFAULT 0,
    next_run_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_outbox_timeline ON outbound_activity_queue(actor_iri, created_at DESC);
CREATE INDEX idx_outbound_queue_process ON outbound_activity_queue(status, next_run_at) WHERE status = 'pending' OR status = 'failed';

-- RDF Dictionary Compression Layer
CREATE TABLE rdf_dictionary (
    id BIGSERIAL PRIMARY KEY,
    value TEXT UNIQUE NOT NULL
);
CREATE INDEX idx_dict_value ON rdf_dictionary(value);

-- Immutable Event Sourced Named Graphs Store
CREATE TABLE rdf_graphs (
    id BIGSERIAL PRIMARY KEY,
    activity_id TEXT NOT NULL,
    object_iri TEXT NOT NULL,
    payload JSONB NOT NULL,                      -- Complete point-in-time JSON-LD representation
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_graphs_object ON rdf_graphs(object_iri, created_at DESC);

-- Clustered Relational Quad Store (S-P-O-G Layout)
CREATE TABLE rdf_quads (
    graph_id BIGINT NOT NULL REFERENCES rdf_graphs(id) ON DELETE CASCADE,
    subject_id BIGINT NOT NULL REFERENCES rdf_dictionary(id),
    predicate_id BIGINT NOT NULL REFERENCES rdf_dictionary(id),
    object_id BIGINT REFERENCES rdf_dictionary(id),
    is_literal BOOLEAN DEFAULT FALSE,
    literal_value TEXT,
    CONSTRAINT chk_quad_object CHECK (
        (is_literal = TRUE AND literal_value IS NOT NULL AND object_id IS NULL) OR
        (is_literal = FALSE AND literal_value IS NULL AND object_id IS NOT NULL)
    )
);
CREATE INDEX idx_quads_sp ON rdf_quads (graph_id, subject_id, predicate_id);
CREATE INDEX idx_quads_op ON rdf_quads (graph_id, object_id, predicate_id);
CREATE UNIQUE INDEX idx_quads_non_literal_uniq ON rdf_quads (graph_id, subject_id, predicate_id, object_id) WHERE object_id IS NOT NULL;
CREATE UNIQUE INDEX idx_quads_literal_uniq ON rdf_quads (graph_id, subject_id, predicate_id, md5(literal_value)) WHERE object_id IS NULL;

-- Global unique registry for physical media assets stored in the central MinIO bucket.
CREATE TABLE media_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    object_name VARCHAR(512) NOT NULL UNIQUE,    -- Safe unique path tracking key in MinIO
    original_name VARCHAR(512) NOT NULL,         -- Stored original filename
    sha256_hex CHAR(64) NOT NULL,                -- Cryptographic content fingerprint signature
    content_type VARCHAR(255) NOT NULL,
    file_size BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for instant global deduplication checks under load
CREATE INDEX idx_media_dedup_hash ON media_attachments(sha256_hex);

-- Connects a media asset to an immutable point-in-time RDF named graph version.
CREATE TABLE rdf_graph_attachments (
    graph_id BIGINT NOT NULL REFERENCES rdf_graphs(id) ON DELETE CASCADE,
    media_attachment_id UUID NOT NULL REFERENCES media_attachments(id) ON DELETE CASCADE,
    PRIMARY KEY (graph_id, media_attachment_id)
);
CREATE INDEX idx_graph_attachments_lookup ON rdf_graph_attachments(media_attachment_id);

-- Enforces multi-tenant storage metrics, accounting, and quota boundaries per local actor.
CREATE TABLE actor_media_ownership (
    actor_iri TEXT NOT NULL,
    tenant_id INT NOT NULL REFERENCES server_tenants(id) ON DELETE CASCADE,
    media_attachment_id UUID NOT NULL REFERENCES media_attachments(id) ON DELETE CASCADE,
    object_name TEXT NOT NULL UNIQUE,
    file_size BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (actor_iri, media_attachment_id)
);
CREATE INDEX idx_actor_media_tenant_quota ON actor_media_ownership(tenant_id, actor_iri);
CREATE INDEX idx_media_ownership_tenant_actor ON actor_media_ownership(tenant_id, actor_iri);

-- Create the rdf_statements view to map universal quads to local multi-tenant partitions implicitly by domain
CREATE VIEW rdf_statements AS
SELECT
    d_sub.value AS subject,
    d_pred.value AS predicate,
    COALESCE(d_obj.value, q.literal_value)::TEXT AS object,
    q.is_literal,
    st.id AS tenant_id,
    q.subject_id,
    q.predicate_id,
    q.object_id,
    q.literal_value AS object_literal
FROM rdf_quads q
JOIN rdf_dictionary d_sub ON q.subject_id = d_sub.id
JOIN rdf_dictionary d_pred ON q.predicate_id = d_pred.id
LEFT JOIN rdf_dictionary d_obj ON q.object_id = d_obj.id
JOIN server_tenants st ON lower(substring(d_sub.value from 'https?://([^/]+)')) = lower(st.domain_name);
