package portstub

import (
	"context"
	"io"

	"sprezz/internal/domain/port"
)

type UnimplementedMediaStoragePort struct{}

var _ port.MediaStoragePort = (*UnimplementedMediaStoragePort)(nil)

func (UnimplementedMediaStoragePort) PutObject(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error) {
	return "", "", nil
}

func (UnimplementedMediaStoragePort) DeleteObject(ctx context.Context, objectName string) error {
	return nil
}
