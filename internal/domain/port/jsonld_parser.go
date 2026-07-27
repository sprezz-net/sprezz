package port

import (
	"context"
	"sprezz/internal/domain/model"
)

// JSONLDParserPort abstracts the transformation and normalization of JSON-LD data.
// It translates ActivityPub and Nomad JSON payloads into flat semantic triple/quad matrices.
type JSONLDParserPort interface {
	// ToQuads processes an incoming byte slice, expanding and normalizing JSON-LD nodes
	// into a stable slice of model.Quad primitives linked to a specific graph identification block.
	ToQuads(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error)
}
