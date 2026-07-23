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

func NewMinIOStorageAdapter(endpoint, accessKey, secretKey, bucketName string, useSSL bool) (*MinIOStorageAdapter, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MinIO client: %w", err)
	}

	return &MinIOStorageAdapter{
		client:     client,
		bucketName: bucketName,
	}, nil
}

var _ ports.MediaStoragePort = (*MinIOStorageAdapter)(nil)

func (m *MinIOStorageAdapter) PutObject(ctx context.Context, objectName string, reader io.Reader, objectSize int64, contentType string) (string, error) {
	exists, err := m.client.BucketExists(ctx, m.bucketName)
	if err != nil {
		return "", fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		err = m.client.MakeBucket(ctx, m.bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to create bucket %s: %w", m.bucketName, err)
		}
	}

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
