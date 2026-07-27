package portstub

import (
	"context"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
)

type UnimplementedJSONLDParserPort struct{}

var _ port.JSONLDParserPort = (*UnimplementedJSONLDParserPort)(nil)

func (UnimplementedJSONLDParserPort) ToQuads(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error) {
	return nil, nil
}
