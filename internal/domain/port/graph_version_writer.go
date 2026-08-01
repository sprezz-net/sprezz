package port

import (
	"context"

	"sprezz/internal/domain/model"
)

// GraphVersionWriter orchestrates multi-table atomicity within a single pgx block.
type GraphVersionWriter interface {
	SaveGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error
	SaveGraphVersionWithMedia(ctx context.Context, params MediaAttachmentParams) error
}

// MediaAttachmentParams unifies parameters needed to anchor a media blob to an RDF graph.
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
