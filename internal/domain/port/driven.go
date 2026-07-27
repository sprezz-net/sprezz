package port

import (
	"context"
	"io"
	"time"

	"sprezz/internal/domain/model"
)

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
	GetCollectionPayloads(ctx context.Context, actorIRI, collection string, limit, offset int) ([][]byte, error)

	GetActorIRIByUsername(ctx context.Context, tenantID int32, username string) (string, error)
	// GetActorProfileFromGraph searches the quad store matching the tenant ID and username handle,
	// returning a unified profile structure parsed directly out of the RDF edges.
	GetActorProfileFromGraph(ctx context.Context, tenantID int32, username string) (*model.ActorProfile, error)
	GetActorProfileByIRI(ctx context.Context, tenantID int32, iri string) (*model.ActorProfile, error)

	ArchiveKeyHistory(ctx context.Context, actorIRI string, keyType string, publicKeyPEM string, validFrom time.Time, validTo time.Time) error
	GetHistoricalKey(ctx context.Context, actorIRI string, keyType string, signedAt time.Time) (string, error)
}

// MediaStoragePort defines the driven port for federated media object storage.
// Media execution is decoupled from RDF persistence to guarantee that a failed
// media stream upload leaves core database nodes entirely untouched.
type MediaStoragePort interface {
	// PutObject streams data into the central bucket and returns (objectKey, sha256Hex, error)
	PutObject(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error)
	DeleteObject(ctx context.Context, objectName string) error
}

type JSONLDParserPort interface {
	ToQuads(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error)
}

// GraphVersionWriter extends the default storage capabilities to process unified,
// multi-table batch operations within a single, context-bound pgx transaction block.
type GraphVersionWriter interface {
	SaveGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error

	// SaveGraphVersionWithMedia registers the central media file, tracks tenant storage metrics,
	// and persists the immutable graph payload within a single atomic database operation.
	SaveGraphVersionWithMedia(ctx context.Context, params MediaAttachmentParams) error
}

type OutboundDispatcher interface {
	ForwardFederatedActivity(ctx context.Context, targetInbox, actorKeyID, privateKeyPEM string, payload []byte) error
}

// MediaAttachmentParams unifies all relational parameters required to link the
// central MinIO media object to specific local multi-tenant actors and graph versions.
type MediaAttachmentParams struct {
	ObjectName   string
	OriginalName string
	SHA256Hex    string
	ContentType  string
	FileSize     int64
	TenantID     string
	ActorIRI     string
	ActivityIRI  string
	ObjectIRI    string
	Payload      []byte
	Quads        []model.Quad
}
