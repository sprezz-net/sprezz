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

	"github.com/gojuno/minimock/v3"
	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/domain/service"
)

// createMultipartRequest builds an in-memory body with structured file chunks and JSON-LD fields.
func createMultipartRequest(t *testing.T, activityJSON string, filenames []string, fileContents []string) (string, *bytes.Buffer) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 1. Append the ActivityPub structural parameter segment
	if activityJSON != "" {
		err := writer.WriteField("activity", activityJSON)
		if err != nil {
			t.Fatalf("failed to write multipart form activity metadata field: %v", err)
		}
	}

	// Loop over all provided attachments to mock array parameters
	for i, filename := range filenames {
		part, err := writer.CreateFormFile("attachment", filename)
		if err != nil {
			t.Fatalf("failed to create multipart form attachment file boundary: %v", err)
		}
		if _, err = io.Copy(part, strings.NewReader(fileContents[i])); err != nil {
			t.Fatalf("failed to populate multipart form binary file content data: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to seal multipart structural write boundaries: %v", err)
	}

	return writer.FormDataContentType(), body
}

func TestMediaUploadHandler_ServeHTTP_Success(t *testing.T) {
	mc := minimock.NewController(t)

	activityPayload := `{"id":"https://sprezz.net","type":"Create","object":"https://sprezz.net"}`

	// Provision multiple file items to traverse the loop context path
	filenames := []string{"vacation.jpg", "document.pdf"}
	contents := []string{"fake-binary-image-data", "fake-pdf-stream"}
	contentType, body := createMultipartRequest(t, activityPayload, filenames, contents)

	// Instantiate mock domain boundaries
	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.VerifyIncomingQuotaMock.Return(true, nil)
	mockStorage.SaveGraphVersionWithMediaMock.Set(func(ctx context.Context, params port.MediaAttachmentParams) error {
		return nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error) {
		return nil, nil
	})

	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockMedia.PutObjectMock.Set(func(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error) {
		return objectName, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", nil
	})

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
	handler := inhttp.NewMediaUploadHandler(svc, 10*1024*1024)

	req := httptest.NewRequest(http.MethodPost, "/media/upload", body)
	req.Header.Set("Content-Type", contentType)

	// Inject upstream identity values utilizing type-safe constant enumerations
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
	mc := minimock.NewController(t)

	filenames := []string{"photo.png"}
	contents := []string{"bytes"}
	contentType, body := createMultipartRequest(t, `{"id":"123"}`, filenames, contents)

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
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

func TestMediaUploadHandler_ServeHTTP_LoopRollbackOnFailure(t *testing.T) {
	mc := minimock.NewController(t)

	activityPayload := `{"id":"https://sprezz.net","type":"Create","object":"https://sprezz.net"}`
	filenames := []string{"first_file.jpg", "second_file.png"}
	contents := []string{"bytes1", "bytes2"}
	contentType, body := createMultipartRequest(t, activityPayload, filenames, contents)

	purgedKeys := make(map[string]bool)
	uploadCount := 0

	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockMedia.PutObjectMock.Set(func(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error) {
		uploadCount++
		if uploadCount == 2 {
			// Simulate infrastructure network or storage block issue on item 2
			return "", "", errors.New("minio node disconnected")
		}
		return objectName, "checksum", nil
	})
	mockMedia.DeleteObjectMock.Set(func(ctx context.Context, objectName string) error {
		purgedKeys[objectName] = true
		return nil
	})

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.VerifyIncomingQuotaMock.Return(true, nil)
	mockStorage.SaveGraphVersionWithMediaMock.Set(func(ctx context.Context, params port.MediaAttachmentParams) error {
		return nil
	})
	mockStorage.RemoveMediaRecordMock.Return(nil)

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error) {
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
	handler := inhttp.NewMediaUploadHandler(svc, 10*1024*1024)

	req := httptest.NewRequest(http.MethodPost, "/media/upload", body)
	req.Header.Set("Content-Type", contentType)

	ctx := context.WithValue(req.Context(), model.TenantIDKey, "tenant-alpha")
	ctx = context.WithValue(ctx, model.ActorIRIKey, "https://sprezz.net")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected internal server error on loop interruption, got %d", rr.Code)
	}

	// Verify that the first key was tracked and cleaned up on rollback
	if len(purgedKeys) == 0 {
		t.Error("Compensating cleanup loop failed to remove previously committed keys after mid-loop abort")
	}
}

func TestMediaUploadHandler_ServeHTTP_MissingActivity(t *testing.T) {
	mc := minimock.NewController(t)

	filenames := []string{"photo.png"}
	contents := []string{"bytes"}
	contentType, body := createMultipartRequest(t, "", filenames, contents) // Empty activity parameter

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
	handler := inhttp.NewMediaUploadHandler(svc, 10*1024*1024)

	req := httptest.NewRequest(http.MethodPost, "/media/upload", body)
	req.Header.Set("Content-Type", contentType)

	// Inject upstream identity values utilizing type-safe constant enumerations
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
	mc := minimock.NewController(t)

	activityPayload := `{"id":"https://sprezz.net","type":"Create","object":"https://sprezz.net"}`
	filenames := []string{"document.pdf"}
	contents := []string{"pdf-stream"}
	contentType, body := createMultipartRequest(t, activityPayload, filenames, contents)

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.VerifyIncomingQuotaMock.Return(true, nil)
	mockStorage.SaveGraphVersionWithMediaMock.Set(func(ctx context.Context, params port.MediaAttachmentParams) error {
		// Trigger a structural relational failure abort
		return errors.New("unique constraint violation on indexes")
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error) {
		return nil, nil
	})

	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockMedia.PutObjectMock.Set(func(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error) {
		return objectName, "checksum", nil
	})
	mockMedia.DeleteObjectMock.Set(func(ctx context.Context, objectName string) error {
		return nil
	})

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)
	handler := inhttp.NewMediaUploadHandler(svc, 10*1024*1024)

	req := httptest.NewRequest(http.MethodPost, "/media/upload", body)
	req.Header.Set("Content-Type", contentType)

	// Inject upstream identity values utilizing type-safe constant enumerations
	ctx := context.WithValue(req.Context(), model.TenantIDKey, "tenant-alpha")
	ctx = context.WithValue(ctx, model.ActorIRIKey, "https://sprezz.net")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d (Internal Server Error) on backend crash, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestProcessInboundMediaTask_Success(t *testing.T) {
	mc := minimock.NewController(t)

	ctx := context.Background()
	putInvoked := false
	saveWithMediaInvoked := false

	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockMedia.PutObjectMock.Set(func(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error) {
		putInvoked = true
		return "permanent/key-123", "hash-sha256-string", nil
	})

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.VerifyIncomingQuotaMock.Return(true, nil)
	mockStorage.SaveGraphVersionWithMediaMock.Set(func(ctx context.Context, params port.MediaAttachmentParams) error {
		saveWithMediaInvoked = true
		if params.ObjectName != "permanent/key-123" || params.SHA256Hex != "hash-sha256-string" {
			t.Errorf("Mismatched metadata mapping values inside parameters context shape: %+v", params)
		}
		return nil
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, rawJSON []byte) ([]model.Quad, error) {
		return []model.Quad{{GraphID: graphID, Subject: "obj", Predicate: "pred", Object: "val"}}, nil
	})

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)

	mediaCtx := port.InboundMediaContext{
		TenantID:     "tenant-1",
		ActorIRI:     "https://sprezz.net",
		ObjectName:   "tmp/upload-id",
		OriginalName: "photo.jpg",
		ContentType:  "image/jpeg",
		Size:         1024,
		MediaStream:  strings.NewReader("dummy-bytes"),
	}

	task := model.InboundTask{
		ID:          "task-1",
		ActivityIRI: "https://sprezz.net",
		ObjectIRI:   "https://sprezz.net",
		Payload:     []byte(`{}`),
	}

	if err := svc.ProcessInboundMediaTask(ctx, mediaCtx, task); err != nil {
		t.Fatalf("Expected successful execution of individual media stream iteration block, got error: %v", err)
	}

	if !putInvoked || !saveWithMediaInvoked {
		t.Error("Pipeline sequences skipped out bounds object storage processing or database commit adapters")
	}
}

func TestProcessInboundMediaTask_StorageCommitFailure(t *testing.T) {
	mc := minimock.NewController(t)

	ctx := context.Background()
	deleteInvokedWithKey := ""

	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockMedia.PutObjectMock.Set(func(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error) {
		return "permanent/isolated-key", "checksum", nil
	})
	mockMedia.DeleteObjectMock.Set(func(ctx context.Context, objectName string) error {
		deleteInvokedWithKey = objectName
		return nil
	})

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockStorage.VerifyIncomingQuotaMock.Return(true, nil)
	mockStorage.SaveGraphVersionWithMediaMock.Set(func(ctx context.Context, params port.MediaAttachmentParams) error {
		return errors.New("simulated postgres context deadlock isolation failure")
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error) {
		return nil, nil
	})

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)

	mediaCtx := port.InboundMediaContext{
		ObjectName:  "tmp/failed-task",
		MediaStream: strings.NewReader("bytes"),
	}
	task := model.InboundTask{Payload: []byte(`{}`)}

	err := svc.ProcessInboundMediaTask(ctx, mediaCtx, task)
	if err == nil {
		t.Fatal("Expected functional bubble up error from internal service tracking due to database failure context, got nil")
	}

	if deleteInvokedWithKey != "permanent/isolated-key" {
		t.Errorf("Compensating rollback mechanism failed to purge un-indexed asset file entry from object storage node. Got key: %q", deleteInvokedWithKey)
	}
}

func TestPurgeOrphanedMedia_Success(t *testing.T) {
	ctx := context.Background()
	mc := minimock.NewController(t)

	targetKey := "tmp/target-to-purge"
	deleteInvoked := false
	removeRecordInvoked := false

	// 1. Stub the Media Storage Mock capability using minimock builder methods
	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockMedia.DeleteObjectMock.Set(func(ctx context.Context, objectName string) error {
		if objectName == targetKey {
			deleteInvoked = true
		}
		return nil
	})

	// 2. Stub the missing Storage Port mock method to prevent unexpected call alerts
	mockStorage := portmock.NewStoragePortMock(mc)
	mockStorage.RemoveMediaRecordMock.Inspect(func(ctx context.Context, objectName string) {
		if objectName == targetKey {
			removeRecordInvoked = true
		}
	}).Return(nil)

	// Initialize the domain coordinator execution service with type-safe mock nodes
	svc := service.NewActivityService(mockStorage, portmock.NewJSONLDParserPortMock(mc), mockMedia)

	// Execute execution routine target
	err := svc.PurgeOrphanedMedia(ctx, targetKey)
	if err != nil {
		t.Fatalf("Unexpected operational failure during explicit cleanup call: %v", err)
	}

	// 3. Verify assertions inside the test block boundaries
	if !deleteInvoked {
		t.Error("Compensating domain action failed to propagate target deletion instruction into object store ports")
	}
	if !removeRecordInvoked {
		t.Error("Compensating loop execution failed to invoke relational database row pruning hook")
	}
}

func TestMediaUploadHandler_ServeHTTP_QuotaCeilingBreached(t *testing.T) {
	activityPayload := `{"id":"https://sprezz.net","type":"Create","object":"https://sprezz.net"}`
	filenames := []string{"oversized_photo.png"}
	contents := []string{"binary-stream"}
	contentType, body := createMultipartRequest(t, activityPayload, filenames, contents)

	mc := minimock.NewController(t)

	// Configure the service stub to simulate an immediate validation failure
	mockSvc := portmock.NewActivityServicePortMock(mc)
	mockSvc.ProcessInboundMediaTaskMock.Set(func(ctx context.Context, mediaCtx port.InboundMediaContext, task model.InboundTask) error {
		return errors.New("storage authorization ceiling threshold exceeded")
	})

	handler := inhttp.NewMediaUploadHandler(mockSvc, 10*1024*1024)

	req := httptest.NewRequest(http.MethodPost, "/media/upload", body)
	req.Header.Set("Content-Type", contentType)

	// Safe SA1029 context configuration bindings
	ctx := context.WithValue(req.Context(), model.TenantIDKey, "tenant-alpha")
	ctx = context.WithValue(ctx, model.ActorIRIKey, "https://sprezz.net")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Verify that the adapter transforms the core restriction into an explicit HTTP 500 or 413 error
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected internal server error code wrapper block on pipeline blockages, got %d", rr.Code)
	}
}
