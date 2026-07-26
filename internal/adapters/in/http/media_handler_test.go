package http_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports"
	"sprezz/internal/domain/ports/portstest"
	"sprezz/internal/domain/service"
)

// Ensure compile-time adherence to both target interface contracts
var _ ports.StoragePort = (*MockStorageAdapter)(nil)
var _ ports.GraphVersionWriter = (*MockStorageAdapter)(nil)

// MockStorageAdapter implements ports.StoragePort and ports.GraphVersionWriter for isolation testing.
type MockStorageAdapter struct {
	portstest.UnimplementedStoragePort // Composite fallback embedded stub
	OnSaveGraphVersion                 func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error
	OnSaveGraphVersionWithMedia        func(ctx context.Context, params ports.MediaAttachmentParams) error
}

// SaveGraphVersion satisfies ports.GraphVersionWriter
func (m *MockStorageAdapter) SaveGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
	if m.OnSaveGraphVersion != nil {
		return m.OnSaveGraphVersion(ctx, activityIRI, objectIRI, payload, quads)
	}
	return nil
}

// SaveGraphVersionWithMedia satisfies ports.GraphVersionWriter
func (m *MockStorageAdapter) SaveGraphVersionWithMedia(ctx context.Context, params ports.MediaAttachmentParams) error {
	if m.OnSaveGraphVersionWithMedia != nil {
		return m.OnSaveGraphVersionWithMedia(ctx, params)
	}
	return nil
}

// MockParserAdapter implements ports.JSONLDParserPort for isolation testing.
type MockParserAdapter struct {
	portstest.UnimplementedJSONLDParserPort // Embedded shared base stub (de-bloated layout)
}

func (m *MockParserAdapter) ToQuads(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error) {
	return []model.Quad{}, nil
}

// MockMediaAdapter implements ports.MediaStoragePort for isolation testing.
type MockMediaAdapter struct {
	portstest.UnimplementedMediaStoragePort // Embedded shared base stub (de-bloated layout)
	OnPutObject                             func(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error)
}

func (m *MockMediaAdapter) PutObject(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error) {
	if m.OnPutObject != nil {
		return m.OnPutObject(ctx, objectName, reader, contentType)
	}
	return objectName, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", nil
}

// createMultipartRequest builds an in-memory body with structured file chunks and JSON-LD fields.
func createMultipartRequest(t *testing.T, activityJSON, filename, fileContent string) (string, *bytes.Buffer) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 1. Append the ActivityPub structural parameter segment
	if activityJSON != "" {
		err := writer.WriteField("activity", activityJSON)
		if err != nil {
			t.Fatalf("failed to write multipart form activity metadata field: %v", err)
		}
	}

	// 2. Append the binary file chunk descriptor segment
	if filename != "" {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("failed to create multipart form attachment file boundary: %v", err)
		}
		_, err = io.Copy(part, strings.NewReader(fileContent))
		if err != nil {
			t.Fatalf("failed to populate multipart form binary file content data: %v", err)
		}
	}

	err := writer.Close()
	if err != nil {
		t.Fatalf("failed to seal multipart structural write boundaries: %v", err)
	}

	return writer.FormDataContentType(), body
}

func TestMediaUploadHandler_ServeHTTP_Success(t *testing.T) {
	activityPayload := `{"id":"https://sprezz.net","type":"Create","object":"https://sprezz.net"}`
	contentType, body := createMultipartRequest(t, activityPayload, "vacation.jpg", "fake-binary-image-data-stream")

	// Instantiate mock domain boundaries
	mockStorage := &MockStorageAdapter{}
	mockParser := &MockParserAdapter{}
	mockMedia := &MockMediaAdapter{}

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
	handler := inhttp.NewMediaUploadHandler(svc, 10*1024*1024)

	req := httptest.NewRequest(http.MethodPost, "/media/upload", body)
	req.Header.Set("Content-Type", contentType)

	// Inject upstream identity values utilizing type-safe constant enumerations (SA1029 Fix)
	ctx := context.WithValue(req.Context(), model.TenantIDKey, "tenant-alpha")
	ctx = context.WithValue(ctx, model.ActorIRIKey, "https://sprezz.net")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("Expected status code %d (Accepted), got %d. Response: %s", http.StatusAccepted, rr.Code, rr.Body.String())
	}

	expectedSubstring := `"status":"committed"`
	if !strings.Contains(rr.Body.String(), expectedSubstring) {
		t.Errorf("Expected response body byte output matrix to contain %s, got %s", expectedSubstring, rr.Body.String())
	}
}

func TestMediaUploadHandler_ServeHTTP_MissingContext(t *testing.T) {
	contentType, body := createMultipartRequest(t, `{"id":"123"}`, "photo.png", "bytes")

	svc := service.NewActivityService(&MockStorageAdapter{}, &MockParserAdapter{}, &MockMediaAdapter{})
	handler := inhttp.NewMediaUploadHandler(svc, 10*1024*1024)

	req := httptest.NewRequest(http.MethodPost, "/media/upload", body)
	req.Header.Set("Content-Type", contentType)

	// Leave the context blocks empty to trigger identity protection branches
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d (Unauthorized) for unmapped context paths, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestMediaUploadHandler_ServeHTTP_MissingActivity(t *testing.T) {
	contentType, body := createMultipartRequest(t, "", "photo.png", "bytes") // Empty activity parameter

	svc := service.NewActivityService(&MockStorageAdapter{}, &MockParserAdapter{}, &MockMediaAdapter{})
	handler := inhttp.NewMediaUploadHandler(svc, 10*1024*1024)

	req := httptest.NewRequest(http.MethodPost, "/media/upload", body)
	req.Header.Set("Content-Type", contentType)

	// Inject upstream identity values utilizing type-safe constant enumerations (SA1029 Fix)
	ctx := context.WithValue(req.Context(), model.TenantIDKey, "tenant-alpha")
	ctx = context.WithValue(ctx, model.ActorIRIKey, "https://sprezz.net")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d (Bad Request) for missing activity parameter, got %d", http.StatusBadRequest, rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "Missing structural 'activity'") {
		t.Errorf("Expected missing activity descriptive string error message payload, got %s", rr.Body.String())
	}
}

func TestMediaUploadHandler_ServeHTTP_DomainProcessingFailure(t *testing.T) {
	activityPayload := `{"id":"https://sprezz.net","type":"Create","object":"https://sprezz.net"}`
	contentType, body := createMultipartRequest(t, activityPayload, "document.pdf", "pdf-stream")

	mockStorage := &MockStorageAdapter{
		OnSaveGraphVersionWithMedia: func(ctx context.Context, params ports.MediaAttachmentParams) error {
			// Trigger a structural relational failure abort
			return errors.New("unique constraint violation on indexes")
		},
	}
	mockParser := &MockParserAdapter{}
	mockMedia := &MockMediaAdapter{}

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
	handler := inhttp.NewMediaUploadHandler(svc, 10*1024*1024)

	req := httptest.NewRequest(http.MethodPost, "/media/upload", body)
	req.Header.Set("Content-Type", contentType)

	// Inject upstream identity values utilizing type-safe constant enumerations (SA1029 Fix)
	ctx := context.WithValue(req.Context(), model.TenantIDKey, "tenant-alpha")
	ctx = context.WithValue(ctx, model.ActorIRIKey, "https://sprezz.net")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d (Internal Server Error) on backend crash, got %d", http.StatusInternalServerError, rr.Code)
	}
}
