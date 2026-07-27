package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/domain/service"

	"github.com/google/uuid"
)

// MediaUploadHandler orchestrates high-performance multi-part incoming attachment streaming.
type MediaUploadHandler struct {
	activitySvc *service.ActivityService
	maxFileSize int64 // Global request size threshold limit
}

// NewMediaUploadHandler instantiates the multi-part streaming request driver.
func NewMediaUploadHandler(svc *service.ActivityService, maxMemoryLimit int64) *MediaUploadHandler {
	return &MediaUploadHandler{
		activitySvc: svc,
		maxFileSize: maxMemoryLimit,
	}
}

// ServeHTTP extracts attachments and activity payloads safely over the wire.
func (h *MediaUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Constrain memory footprint spike limits
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.writeError(w, http.StatusBadRequest, "Malformed multipart payload")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	tenantID, _ := ctx.Value(model.TenantIDKey).(string)
	actorIRI, _ := ctx.Value(model.ActorIRIKey).(string)
	if tenantID == "" || actorIRI == "" {
		h.writeError(w, http.StatusUnauthorized, "Missing tenant or identity boundaries")
		return
	}

	// Extract the ActivityPub event payload from the multipart request
	activityJSON := r.FormValue("activity")
	// Explicitly catch empty string payloads before unmarshaling to prevent JSON parsing errors
	if activityJSON == "" {
		h.writeError(w, http.StatusBadRequest, "Missing structural 'activity' parameter metadata")
		return
	}
	var envelope struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	}
	if err := json.Unmarshal([]byte(activityJSON), &envelope); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid Activity metadata format")
		return
	}

	// 2. Fetch the collection array instead of a single form file
	files := r.MultipartForm.File["attachment"]
	if len(files) == 0 {
		h.writeError(w, http.StatusBadRequest, "No attachments found under 'attachment' key")
		return
	}

	var completedObjectKeys []string

	// 3. Sequential Multi-File Streaming Loop
	for _, fileHeader := range files {
		fileStream, err := fileHeader.Open()
		if err != nil {
			h.executeCompensatingCleanup(completedObjectKeys)
			h.writeError(w, http.StatusBadRequest, "Failed to read file chunk stream")
			return
		}

		// Handle both return values from uuid.NewV7()
		taskUUID, err := uuid.NewV7()
		if err != nil {
			_ = fileStream.Close()
			h.executeCompensatingCleanup(completedObjectKeys)
			h.writeError(w, http.StatusInternalServerError, "Failed to generate unique task identifier")
			return
		}

		taskID := taskUUID.String()
		tempObjectKey := fmt.Sprintf("tmp/%s", taskID)

		// Pack parameters into the single structure matching your exact service parameters
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
			ActivityIRI: envelope.ID,
			ObjectIRI:   envelope.Object,
			Payload:     []byte(activityJSON),
		}

		// Process single iteration unit
		if err := h.activitySvc.ProcessInboundMediaTask(ctx, mediaCtx, task); err != nil {
			_ = fileStream.Close()
			h.executeCompensatingCleanup(completedObjectKeys)
			h.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		completedObjectKeys = append(completedObjectKeys, tempObjectKey)
		_ = fileStream.Close() // Immediate deterministic release
	}

	// 7. Success Manifest Response Writeout
	w.Header().Set(headerContentType, "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"committed"}`))
}

// executeCompensatingCleanup runs a reverse pruning sequence if an iteration fails mid-loop
func (h *MediaUploadHandler) executeCompensatingCleanup(keys []string) {
	for _, key := range keys {
		_ = h.activitySvc.PurgeOrphanedMedia(context.Background(), key)
	}
}

func (h *MediaUploadHandler) writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set(headerContentType, "application/json")
	w.WriteHeader(status)

	// Replaced memory allocation loop using zero-allocation byte block appends
	errorBytes := fmt.Appendf(nil, `{"error":%q}`, msg)
	_, _ = w.Write(errorBytes)
}
