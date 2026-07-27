package portstub

import (
	"context"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
)

type UnimplementedGraphVersionWriter struct{}

var _ port.GraphVersionWriter = (*UnimplementedGraphVersionWriter)(nil)

func (UnimplementedGraphVersionWriter) SaveGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
	return nil
}

func (UnimplementedGraphVersionWriter) SaveGraphVersionWithMedia(ctx context.Context, params port.MediaAttachmentParams) error {
	return nil
}
