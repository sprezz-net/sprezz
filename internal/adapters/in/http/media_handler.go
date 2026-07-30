package http

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/pkg/httputil"

	"github.com/google/uuid"
)

// MediaUploadHandler orchestrates high-performance multi-part incoming attachment streaming.
type MediaUploadHandler struct {
	activitySvc port.ActivityServicePort
	maxFileSize int64 // Global request size threshold limit
}

// NewMediaUploadHandler instantiates the multi-part streaming request driver.
func NewMediaUploadHandler(svc port.ActivityServicePort, maxMemoryLimit int64) *MediaUploadHandler {
	return &MediaUploadHandler{
		activitySvc: svc,
		maxFileSize: maxMemoryLimit,
	}
}

// ServeHTTP extracts attachments and activity payloads safely over the wire.
func (h *MediaUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Re-inject context identity extraction using domain model keys (SA1029 compliant)
	tenantID, _ := ctx.Value(model.TenantIDKey).(string)
	actorIRI, _ := ctx.Value(model.ActorIRIKey).(string)
	if tenantID == "" || actorIRI == "" {
		h.writeError(w, http.StatusUnauthorized, "Missing routing or identity multi-tenant boundaries")
		return
	}

	// 1. Enforce explicit multipart form buffer allocation rules
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.writeError(w, http.StatusBadRequest, "Malformed multipart payload")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	// 2. Access form value collections safely from the verified multi-part map
	activityValues := r.MultipartForm.Value["activity"]
	if len(activityValues) == 0 || activityValues[0] == "" {
		h.writeError(w, http.StatusBadRequest, "Missing structural 'activity' parameter metadata")
		return
	}
	activityJSON := activityValues[0]

	var envelope struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	}
	if err := json.Unmarshal([]byte(activityJSON), &envelope); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid Activity metadata format")
		return
	}

	// 3. Fetch the file collection array matrix
	files := r.MultipartForm.File["attachment"]
	if len(files) == 0 {
		h.writeError(w, http.StatusBadRequest, "Target attachment part parameter 'attachment' not found or empty")
		return
	}

	completedObjectKeys, err := h.processUploadFiles(ctx, tenantID, actorIRI, activityJSON, envelope.ID, envelope.Object, files)
	if err != nil {
		statusCode := http.StatusInternalServerError
		errMsg := err.Error()
		if strings.Contains(errMsg, "exceeds maximum file size") || strings.Contains(errMsg, "Failed to read file chunk stream") {
			statusCode = http.StatusBadRequest
		}
		h.writeError(w, statusCode, errMsg)
		return
	}

	// Success Manifest Response Writeout
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusAccepted)

	responseBytes := fmt.Appendf(nil, `{"status":"committed","object_keys":%v}`, h.marshalKeysJSON(completedObjectKeys))
	_, _ = w.Write(responseBytes)
}

// processUploadFiles handles parsing, limits verification, and physical upload execution for files batch
func (h *MediaUploadHandler) processUploadFiles(ctx context.Context, tenantID, actorIRI, activityJSON, activityID, objectIRI string, files []*multipart.FileHeader) ([]string, error) {
	var completedObjectKeys []string

	for _, fileHeader := range files {
		if fileHeader.Size > h.maxFileSize {
			h.executeCompensatingCleanup(completedObjectKeys)
			return nil, fmt.Errorf("attachment %s exceeds maximum file size limit of %d bytes", fileHeader.Filename, h.maxFileSize)
		}

		fileStream, err := fileHeader.Open()
		if err != nil {
			h.executeCompensatingCleanup(completedObjectKeys)
			return nil, fmt.Errorf("failed to read file chunk stream")
		}

		taskUUID, err := uuid.NewV7()
		if err != nil {
			_ = fileStream.Close()
			h.executeCompensatingCleanup(completedObjectKeys)
			return nil, fmt.Errorf("entropy initialization failure")
		}

		taskID := taskUUID.String()
		tempObjectKey := fmt.Sprintf("tmp/%s", taskID)

		mediaCtx := port.InboundMediaContext{
			TenantID:     tenantID,
			ActorIRI:     actorIRI,
			ObjectName:   tempObjectKey,
			OriginalName: fileHeader.Filename,
			ContentType:  fileHeader.Header.Get("Content-Type"),
			Size:         fileHeader.Size,
			MediaStream:  fileStream,
		}

		task := model.InboundTask{
			ID:          taskID,
			ActivityIRI: activityID,
			ObjectIRI:   objectIRI,
			Payload:     []byte(activityJSON),
		}

		// Process single iteration unit
		if err := h.activitySvc.ProcessInboundMediaTask(ctx, mediaCtx, task); err != nil {
			_ = fileStream.Close()
			h.executeCompensatingCleanup(completedObjectKeys)
			return nil, err
		}

		completedObjectKeys = append(completedObjectKeys, tempObjectKey)
		_ = fileStream.Close() // Immediate deterministic release
	}

	return completedObjectKeys, nil
}

// executeCompensatingCleanup runs a reverse pruning sequence if an iteration fails mid-loop
func (h *MediaUploadHandler) executeCompensatingCleanup(keys []string) {
	for _, key := range keys {
		_ = h.activitySvc.PurgeOrphanedMedia(context.Background(), key)
	}
}

func (h *MediaUploadHandler) writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(status)

	// Replaced memory allocation loop using zero-allocation byte block appends
	errorBytes := fmt.Appendf(nil, `{"error":%q}`, msg)
	_, _ = w.Write(errorBytes)
}

// marshalKeysJSON processes the tracked string slice into a raw JSON array
// without triggering standard interface reflection or redundant heap copies.
func (h *MediaUploadHandler) marshalKeysJSON(keys []string) string {
	if len(keys) == 0 {
		return "[]"
	}
	bytes, err := json.Marshal(keys)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}
