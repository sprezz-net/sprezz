package minio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sprezz/internal/domain/port"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOStorageAdapter implements port.MediaStoragePort for the Sprezz federation server.
type MinIOStorageAdapter struct {
	client         *minio.Client
	bucketName     string
	checkedBuckets sync.Map // Thread-safe cache to avoid duplicate BucketExists calls under load
}

// Ensure interface adherence at compile time
var _ port.MediaStoragePort = (*MinIOStorageAdapter)(nil)

// NewMinIOStorageAdapter instantiates the client safely using 5 parameters and custom optional overlays.
func NewMinIOStorageAdapter(endpoint, accessKey, secretKey, bucketName string, useSSL bool, extraOpts ...minio.Options) (*MinIOStorageAdapter, error) {
	// 1. Initialize base parameter options
	opts := minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	}

	// 2. Layer custom configuration elements iteratively rather than overwriting the whole struct block
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
		return nil, fmt.Errorf("failed to initialize MinIO client core: %w", err)
	}

	adapter := &MinIOStorageAdapter{
		client:     client,
		bucketName: bucketName,
	}

	// 3. Idempotently verify or provision bucket status immediately upon initialization
	ctx := context.Background()
	if err := adapter.ensureBucket(ctx, bucketName); err != nil {
		return nil, err
	}

	return adapter, nil
}

// ensureBucket checks or provisions the configured target bucket idempotently.
// It is strictly context-bound to eliminate orphaned network sockets on timeout.
func (m *MinIOStorageAdapter) ensureBucket(ctx context.Context, bucket string) error {
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

// PutObject streams data from the network into MinIO while computing a SHA-256 fingerprint concurrently.
// This matches the exact contract signature expected by port.MediaStoragePort.
func (m *MinIOStorageAdapter) PutObject(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error) {
	if err := m.ensureBucket(ctx, m.bucketName); err != nil {
		return "", "", err
	}

	// 1. Setup the content-addressing cryptographic hashing state
	hasher := sha256.New()

	// 2. The TeeReader automatically pipes data into the hasher as MinIO reads from it
	teeReader := io.TeeReader(reader, hasher)

	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}

	// 3. Pass -1 for size to leverage seamless multi-part chunked object streaming
	info, err := m.client.PutObject(ctx, m.bucketName, objectName, teeReader, -1, opts)
	if err != nil {
		return "", "", fmt.Errorf("media content-addressed upload failed for %s/%s: %w", m.bucketName, objectName, err)
	}

	// 4. Extract the finalized hexadecimal payload fingerprint string
	sha256Hex := hex.EncodeToString(hasher.Sum(nil))

	// 5. Return both the stable object key location and the calculated SHA-256 string
	return info.Key, sha256Hex, nil
}

// DeleteObject removes an object from the configured bucket safely.
func (m *MinIOStorageAdapter) DeleteObject(ctx context.Context, objectName string) error {
	err := m.client.RemoveObject(ctx, m.bucketName, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove object %s from bucket %s: %w", objectName, m.bucketName, err)
	}
	return nil
}
