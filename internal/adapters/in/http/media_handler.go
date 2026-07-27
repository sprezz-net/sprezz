package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"sprezz/internal/domain/model"
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

	// 1. Constrain memory footprint spikes by applying a strict reading threshold limit
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize)

	// Allocate a safe in-memory parsing buffer maximized at 32MB
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.writeError(w, http.StatusBadRequest, "Payload exceeds authorized threshold or is malformed")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	// 2. Extract tenant and actor routing metadata using type-safe context key constants
	tenantID, _ := ctx.Value(model.TenantIDKey).(string)
	actorIRI, _ := ctx.Value(model.ActorIRIKey).(string)
	if tenantID == "" || actorIRI == "" {
		h.writeError(w, http.StatusUnauthorized, "Missing routing or identity multi-tenant boundaries")
		return
	}

	// 3. Isolate the associated ActivityPub event payload embedded within the multi-part body
	activityJSON := r.FormValue("activity")
	if activityJSON == "" {
		h.writeError(w, http.StatusBadRequest, "Missing structural 'activity' parameter metadata")
		return
	}

	var tempEnvelope struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Object string `json:"object"`
	}
	if err := json.Unmarshal([]byte(activityJSON), &tempEnvelope); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid JSON-LD structural payload format")
		return
	}

	// 4. Retrieve variable file attachments
	multipartForm := r.MultipartForm
	files := multipartForm.File["attachment"] // Explicit array field lookup
	if len(files) == 0 {
		h.writeError(w, http.StatusBadRequest, "Target attachment part parameter 'attachment' not found or empty")
		return
	}

	// Array to track generated object keys for potential compensating cleanups
	var completedObjectKeys []string

	// 5. The Multipart Media Form Attachment Upload Loop
	// Iterates sequentially over incoming streams to preserve tight memory boundaries
	for _, fileHeader := range files {

		// Open the file stream for the individual chunk iteration
		fileStream, err := fileHeader.Open()
		if err != nil {
			h.executeCompensatingCleanup(completedObjectKeys)
			h.writeError(w, http.StatusBadRequest, "Failed to read file chunk stream")
			return
		}

		// Generate a unique, time-ordered sequential UUIDv7 token for this attachment
		taskID, err := uuid.NewV7()
		if err != nil {
			_ = fileStream.Close()
			h.executeCompensatingCleanup(completedObjectKeys)
			h.writeError(w, http.StatusInternalServerError, "Failed to provision secure task tracking identifier")
			return
		}

		// Dynamic temporary destination key assignment using the unique execution token
		tempObjectKey := fmt.Sprintf("tmp/%s", taskID.String())

		// Pack parameters into the single structure matching your exact service parameters
		mediaCtx := service.InboundMediaContext{
			TenantID:     tenantID,
			ActorIRI:     actorIRI,
			ObjectName:   tempObjectKey,
			OriginalName: fileHeader.Filename,
			ContentType:  fileHeader.Header.Get(headerContentType),
			Size:         fileHeader.Size,
			MediaStream:  fileStream,
		}

		task := model.InboundTask{
			ID:          taskID.String(),
			ActivityIRI: tempEnvelope.ID,
			ObjectIRI:   tempEnvelope.Object,
			Payload:     []byte(activityJSON),
		}

		// 6. Delegate processing down to the domain activity execution core
		// Pre-flight metrics and hard ceiling validations execute inline within this call
		if err := h.activitySvc.ProcessInboundMediaTask(ctx, mediaCtx, task); err != nil {
			_ = fileStream.Close()
			h.executeCompensatingCleanup(completedObjectKeys)
			h.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Processing error encountered: %v", err))
			return
		}

		// Append tracked key only after a successful internal domain execution step
		completedObjectKeys = append(completedObjectKeys, tempObjectKey)
		_ = fileStream.Close() // Immediate deterministic socket release per loop iteration
	}

	// 7. Success Manifest Response Writeout
	w.Header().Set(headerContentType, "application/json")
	w.WriteHeader(http.StatusAccepted)

	responseBytes := fmt.Appendf(nil, `{"status":"committed","object_keys":%v}`, h.marshalKeysJSON(completedObjectKeys))
	_, _ = w.Write(responseBytes)
}

// executeCompensatingCleanup runs a reverse pruning sequence if an iteration fails mid-loop (Blueprint 9.2)
func (h *MediaUploadHandler) executeCompensatingCleanup(objectKeys []string) {
	for _, key := range objectKeys {
		_ = h.activitySvc.PurgeOrphanedMedia(context.Background(), key)
	}
}

func (h *MediaUploadHandler) marshalKeysJSON(keys []string) string {
	bytes, err := json.Marshal(keys)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

func (h *MediaUploadHandler) writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set(headerContentType, "application/json")
	w.WriteHeader(status)

	// Replaced memory allocation loop using zero-allocation byte block appends
	errorBytes := fmt.Appendf(nil, `{"error":%q}`, msg)
	_, _ = w.Write(errorBytes)
}
