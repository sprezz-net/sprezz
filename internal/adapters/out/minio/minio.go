package minio

import (
	"context"
	"fmt"
	"io"

	"sprezz/internal/domain/ports"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOStorageAdapter struct {
	client     *minio.Client
	bucketName string
}

// NewMinIOStorageAdapter instantiates the client safely.
func NewMinIOStorageAdapter(endpoint, accessKey, secretKey, bucketName string, useSSL bool, extraOpts ...minio.Options) (*MinIOStorageAdapter, error) {
	// 1. Initialize base parameters
	opts := minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	}

	// FIXED: Layer custom configuration elements iteratively rather than overwriting the whole struct block.
	// This ensures your Transport interceptor is preserved along with standard attributes.
	if len(extraOpts) > 0 {
		provided := extraOpts[0]
		if provided.Transport != nil {
			opts.Transport = provided.Transport
		}
		if provided.BucketLookup != 0 {
			opts.BucketLookup = provided.BucketLookup
		}
		if provided.Creds != nil {
			opts.Creds = provided.Creds
		}
	}

	client, err := minio.New(endpoint, &opts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MinIO client: %w", err)
	}

	adapter := &MinIOStorageAdapter{
		client:     client,
		bucketName: bucketName,
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to verify bucket configuration state: %w", err)
	}
	if !exists {
		err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize media target storage bucket %s: %w", bucketName, err)
		}
	}

	return adapter, nil
}

var _ ports.MediaStoragePort = (*MinIOStorageAdapter)(nil)

func (m *MinIOStorageAdapter) PutObject(ctx context.Context, objectName string, reader io.Reader, objectSize int64, contentType string) (string, error) {
	info, err := m.client.PutObject(ctx, m.bucketName, objectName, reader, objectSize, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload object %s: %w", objectName, err)
	}

	location := fmt.Sprintf("/%s/%s", m.bucketName, info.Key)
	return location, nil
}

func (m *MinIOStorageAdapter) DeleteObject(ctx context.Context, objectName string) error {
	err := m.client.RemoveObject(ctx, m.bucketName, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove object %s: %w", objectName, err)
	}
	return nil
}
