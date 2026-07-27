package port

import (
	"context"
	"io"
)

// MediaStoragePort abstracts binary blob lifecycle operations for federated media.
type MediaStoragePort interface {
	PutObject(ctx context.Context, objectName string, reader io.Reader, contentType string) (objectKey string, sha256Hex string, err error)
	DeleteObject(ctx context.Context, objectName string) error
}
