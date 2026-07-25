package media

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinioHandler implements ports.MediaStoragePort for the Sprezz federation server.
type MinioHandler struct {
	client         *minio.Client
	bucketName     string   // Retains the central bucket target from application configuration
	checkedBuckets sync.Map // Thread-safe cache to avoid duplicate BucketExists calls under load
}

// NewMinioHandler instantiates the production object-storage adapter using 5 parameters.
func NewMinioHandler(endpoint, accessKey, secretKey, bucketName string, useSSL bool) (*MinioHandler, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init minio core client: %w", err)
	}

	return &MinioHandler{
		client:     client,
		bucketName: bucketName,
	}, nil
}

// ensureBucket checks or provisions the configured target bucket idempotently.
// It is strictly context-bound to eliminate orphaned network sockets on timeout.
func (m *MinioHandler) ensureBucket(ctx context.Context, bucket string) error {
	if _, verified := m.checkedBuckets.Load(bucket); verified {
		return nil
	}

	exists, err := m.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("failed bucket verification check for '%s': %w", bucket, err)
	}

	if !exists {
		err = m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("failed to auto-provision target bucket '%s': %w", bucket, err)
		}
	}

	m.checkedBuckets.Store(bucket, true)
	return nil
}

// PutObject pipelines an io.Reader directly into the configured central object storage.
// This fulfills your ports.MediaStoragePort contract cleanly while utilizing the implicit bucket state.
func (m *MinioHandler) PutObject(ctx context.Context, objectName string, reader io.Reader, objectSize int64, contentType string) (string, error) {
	if err := m.ensureBucket(ctx, m.bucketName); err != nil {
		return "", err
	}

	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}

	info, err := m.client.PutObject(ctx, m.bucketName, objectName, reader, objectSize, opts)
	if err != nil {
		return "", fmt.Errorf("media stream upload execution failed for %s/%s: %w", m.bucketName, objectName, err)
	}

	// Returns the stable unique object location back up to your activity service layer
	return info.Key, nil
}

// DeleteObject drops target artifacts from the central configured bucket silently.
// This satisfies your ports.MediaStoragePort interface to execute safe rolling cleanups during failures.
func (m *MinioHandler) DeleteObject(ctx context.Context, objectName string) error {
	err := m.client.RemoveObject(ctx, m.bucketName, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to complete object removal for %s/%s: %w", m.bucketName, objectName, err)
	}
	return nil
}
