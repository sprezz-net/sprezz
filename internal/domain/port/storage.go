package port

import (
	"context"
	"time"

	"sprezz/internal/domain/model"
)

// StoragePort defines the primary relational ledger contract.
type StoragePort interface {
	// Domain Routing & Multi-Tenant Isolation Stubs

	// GetOrCreateTenantByDomain checks for domain presence, inserting it dynamically if missing.
	GetOrCreateTenantByDomain(ctx context.Context, domainName string) (int32, error)

	// HasActorCredential checks if a specific username exists inside a designated tenant partition.
	HasActorCredential(ctx context.Context, tenantID int32, username string) (bool, error)
	GetActorCredentials(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error)

	// CreateActorCredential commits a newly generated cryptographic dual-key identity to long-term storage.
	CreateActorCredential(ctx context.Context, actorIRI string, tenantID int32, username string, privateKeyRSAPEM string, privateKeyEd25519PEM string) error

	IsDomainBlocked(ctx context.Context, domainName string) (bool, error)
	EnqueueInbound(ctx context.Context, id string, activityIRI, objectIRI, targetDomain string, payload []byte) error
	ClaimInboundBatch(ctx context.Context, batchSize int) ([]model.InboundTask, error)
	MarkInboundComplete(ctx context.Context, id string) error
	MarkInboundFailed(ctx context.Context, id string, reason string) error

	// Delivery tracking metric
	RecordActorInboxDelivery(ctx context.Context, actorIRI, activityIRI string) error

	// Nomadic Identity Management
	GetNomadicIdentity(ctx context.Context, guid string) (*model.NomadicIdentity, error)
	UpsertNomadicIdentity(ctx context.Context, identity *model.NomadicIdentity) error
	RegisterIdentityClone(ctx context.Context, guid string, hubURL string, isLocal bool) error
	GetActorDualKeys(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error)

	// Core RDF Event Sourcing Write Operations
	CreateGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte) (int64, error)
	SaveQuads(ctx context.Context, quads []model.Quad) error
	SaveQuadIDs(ctx context.Context, quadIDs []model.QuadID) error
	RemoveQuadEdge(ctx context.Context, subject, predicate, object string) error

	// Core RDF Graph Read Operations
	GetLatestPayload(ctx context.Context, objectIRI string) ([]byte, error)
	StreamQuadsBySubject(ctx context.Context, subjectIRI string) ([]model.Quad, error)
	GetStatementsBySubjectIsolated(ctx context.Context, subjectIRI string, tenantID int32) ([]model.Quad, error)
	GetTenantIDByActivityIRI(ctx context.Context, activityIRI string) (int32, error)
	GetCollectionPayloads(ctx context.Context, actorIRI, collection string, limit, offset int) ([][]byte, error)

	GetActorIRIByUsername(ctx context.Context, tenantID int32, username string) (string, error)
	// GetActorProfileFromGraph searches the quad store matching the tenant ID and username handle,
	// returning a unified profile structure parsed directly out of the RDF edges.
	GetActorProfileFromGraph(ctx context.Context, tenantID int32, username string) (*model.ActorProfile, error)
	GetActorProfileByIRI(ctx context.Context, tenantID int32, iri string) (*model.ActorProfile, error)
	GetActorIRIByAlias(ctx context.Context, alias string) (string, error)

	ArchiveKeyHistory(ctx context.Context, actorIRI string, keyType string, publicKeyPEM string, validFrom time.Time, validTo time.Time) error
	GetHistoricalKey(ctx context.Context, actorIRI string, keyType string, signedAt time.Time) (string, error)

	// VerifyIncomingQuota runs an aggregate space metric scan against hard multi-tenant boundaries.
	// Returns true if the incoming payload allocation size fits cleanly within safety guidelines.
	VerifyIncomingQuota(ctx context.Context, tenantID int32, incomingSizeBytes int64) (bool, error)
	// RemoveMediaRecord explicitly prunes aborted metadata weights to free up multi-tenant space.
	RemoveMediaRecord(ctx context.Context, objectName string) error
}

// StorageAndGraphWriter combines StoragePort and GraphVersionWriter interfaces for testing/mocking convenience.
type StorageAndGraphWriter interface {
	StoragePort
	GraphVersionWriter
}
