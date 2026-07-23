CREATE TYPE activity_status AS ENUM ('pending', 'processing', 'completed', 'failed');

-- Domain Multi-Tenancy Management
CREATE TABLE server_tenants (
    id SERIAL PRIMARY KEY,
    domain_name TEXT UNIQUE NOT NULL
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
    private_key_pem TEXT NOT NULL,
    UNIQUE (tenant_id, username)
);

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
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_outbox_timeline ON outbound_activity_queue(actor_iri, created_at DESC);

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
    object_id BIGINT NOT NULL REFERENCES rdf_dictionary(id),
    is_literal BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (graph_id, subject_id, predicate_id, object_id)
);
CREATE INDEX idx_quads_sp ON rdf_quads (graph_id, subject_id, predicate_id);
CREATE INDEX idx_quads_op ON rdf_quads (graph_id, object_id, predicate_id);
