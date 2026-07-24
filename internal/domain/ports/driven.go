package ports

import (
	"context"
	"io"
	"sprezz/internal/domain/model"
)

type StoragePort interface {
	// Domain Routing & Multi-Tenant Isolation
	IsDomainBlocked(ctx context.Context, domainName string) (bool, error)
	EnqueueInbound(ctx context.Context, id string, activityIRI, objectIRI, targetDomain string, payload []byte) error
	ClaimInboundBatch(ctx context.Context, batchSize int) ([]model.InboundTask, error)
	MarkInboundComplete(ctx context.Context, id string) error
	MarkInboundFailed(ctx context.Context, id string, reason string) error

	// Nomadic Identity Management
	GetNomadicIdentity(ctx context.Context, guid string) (*model.NomadicIdentity, error)
	UpsertNomadicIdentity(ctx context.Context, identity *model.NomadicIdentity) error
	RegisterIdentityClone(ctx context.Context, guid string, hubURL string, isLocal bool) error
	GetActorPrivateKey(ctx context.Context, actorIRI string) (string, error)

	// Core RDF Event Sourcing Write Operations
	CreateGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte) (int64, error)
	SaveQuads(ctx context.Context, quads []model.Quad) error

	// Expose SaveQuadIDs at the port boundary to allow low-latency,
	// zero-allocation index batch writing directly from performance-critical services.
	SaveQuadIDs(ctx context.Context, quadIDs []model.QuadID) error
	RemoveQuadEdge(ctx context.Context, subject, predicate, object string) error

	// Core RDF Graph Read Operations
	GetLatestPayload(ctx context.Context, objectIRI string) ([]byte, error)
	StreamQuadsBySubject(ctx context.Context, subjectIRI string) ([]model.Quad, error)
}

type MediaStoragePort interface {
	PutObject(ctx context.Context, objectName string, reader io.Reader, objectSize int64, contentType string) (string, error)
	DeleteObject(ctx context.Context, objectName string) error
}

type JSONLDParserPort interface {
	ToQuads(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error)
}

type GraphVersionWriter interface {
	SaveGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error
}

type OutboundDispatcher interface {
	ForwardFederatedActivity(ctx context.Context, targetInbox, actorKeyID, privateKeyPEM string, payload []byte) error
}
