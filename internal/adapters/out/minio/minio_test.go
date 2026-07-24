package minio_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"sprezz/internal/adapters/out/minio"
	sdk "github.com/minio/minio-go/v7"
)

type mockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

// matchPutRequest splits the complex PUT routing logic to drop Cognitive Complexity below Sonar's threshold.
func matchPutRequest(req *http.Request, bucket, objectName string) (*http.Response, error) {
	if strings.Contains(req.URL.Path, objectName) {
		respBody := `<?xml version="1.0" encoding="UTF-8"?><CopyObjectResult><ETag>"hash123"</ETag></CopyObjectResult>`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(respBody)),
		}, nil
	}

	isCreationPath := req.URL.Path == "/" || req.URL.Path == "" || strings.HasSuffix(strings.TrimRight(req.URL.Path, "/"), "/"+bucket)
	if isCreationPath {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(""))),
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(bytes.NewReader([]byte("unexpected put route mismatch"))),
	}, nil
}

func TestMinIOStorageAdapter_PutObject_Success(t *testing.T) {
	bucket := "test-bucket"
	objectName := "media/avatar.png"
	payload := []byte("fake-image-bytes")

	clientTransport := &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			// Phase 1: SDK initial Region location probe
			if req.Method == http.MethodGet && strings.Contains(req.URL.RawQuery, "location") {
				respBody := `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://amazonaws.com">us-east-1</LocationConstraint>`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			}

			isTargetingBucket := strings.Contains(req.URL.Host, bucket) || strings.Contains(req.URL.Path, "/"+bucket)

			// Phase 2: BucketExists execution check (HEAD)
			if req.Method == http.MethodHead && isTargetingBucket {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(bytes.NewReader([]byte(""))),
				}, nil
			}

			// Phase 3 & 4: Delegate to our flattened matching function to drop cyclomatic complexity metrics
			if req.Method == http.MethodPut && isTargetingBucket {
				return matchPutRequest(req, bucket, objectName)
			}

			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(bytes.NewReader([]byte("unexpected test request mock route mismatch"))),
			}, nil
		},
	}

	testOpts := sdk.Options{
		Transport:    clientTransport,
		BucketLookup: sdk.BucketLookupPath,
	}

	adapter, err := minio.NewMinIOStorageAdapter("localhost:9000", "mock-user", "mock-pass", bucket, false, testOpts)
	if err != nil {
		t.Fatalf("failed to initialize production storage wrapper structure: %v", err)
	}

	ctx := context.Background()
	location, err := adapter.PutObject(ctx, objectName, bytes.NewReader(payload), int64(len(payload)), "image/png")

	if err != nil {
		t.Fatalf("Expected successful payload storage execution pipeline run, got error: %v", err)
	}

	expectedLocation := "/test-bucket/media/avatar.png"
	if location != expectedLocation {
		t.Errorf("Expected uploaded resource reference key string to format exactly to %q, got %q", expectedLocation, location)
	}
}

func TestMinIOStorageAdapter_DeleteObject_Success(t *testing.T) {
	bucket := "test-bucket"
	objectName := "media/avatar.png"

	clientTransport := &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			isTargetingBucket := strings.Contains(req.URL.Host, bucket) || strings.Contains(req.URL.Path, "/"+bucket)

			if req.Method == http.MethodGet && strings.Contains(req.URL.RawQuery, "location") {
				respBody := `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://amazonaws.com">us-east-1</LocationConstraint>`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			}

			if req.Method == http.MethodHead && isTargetingBucket {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(""))),
				}, nil
			}

			if req.Method == http.MethodDelete && strings.Contains(req.URL.Path, objectName) {
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(bytes.NewReader([]byte(""))),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(bytes.NewReader([]byte("unexpected test request mock route mismatch"))),
			}, nil
		},
	}

	testOpts := sdk.Options{
		Transport:    clientTransport,
		BucketLookup: sdk.BucketLookupPath,
	}

	adapter, err := minio.NewMinIOStorageAdapter("localhost:9000", "mock-user", "mock-pass", bucket, false, testOpts)
	if err != nil {
		t.Fatalf("failed to initialize production storage wrapper structure: %v", err)
	}

	ctx := context.Background()
	err = adapter.DeleteObject(ctx, objectName)
	if err != nil {
		t.Fatalf("Expected successful resource purge context cycle exit, got error: %v", err)
	}
}
