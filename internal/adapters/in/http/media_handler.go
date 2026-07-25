package http

import (
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
	maxFileSize int64
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
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		h.writeError(w, http.StatusBadRequest, "Payload exceeds authorized threshold or is malformed")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	// 2. Extract tenant and actor routing metadata injected by upstream middlewares
	tenantID, _ := ctx.Value("tenant_id").(string)
	actorIRI, _ := ctx.Value("actor_iri").(string)
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

	// 4. Retrieve the file binary body chunk stream from the form part field safely
	file, header, err := r.FormFile("file")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Target attachment part parameter 'file' not found")
		return
	}
	// Discard error explicitly within a function literal to satisfy errcheck metrics
	defer func() { _ = file.Close() }()

	// 5. Generate a unique, time-ordered sequential UUIDv7 token for this temporary streaming path
	taskID, err := uuid.NewV7()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "Failed to provision secure task tracking identifier")
		return
	}

	// Dynamic path assignment using the unique task execution token
	tempObjectKey := fmt.Sprintf("tmp/%s", taskID.String())

	// Structure execution blocks into a unified context token to comply with the 7-parameter function limit
	mediaCtx := service.InboundMediaContext{
		TenantID:     tenantID,
		ActorIRI:     actorIRI,
		ObjectName:   tempObjectKey,
		OriginalName: header.Filename,
		ContentType:  header.Header.Get(headerContentType),
		Size:         header.Size,
		MediaStream:  file,
	}

	task := model.InboundTask{
		ID:          taskID.String(),
		ActivityIRI: tempEnvelope.ID,
		ObjectIRI:   tempEnvelope.Object,
		Payload:     []byte(activityJSON),
	}

	// 6. Delegate the pipeline processing down to the domain activity execution core
	if err := h.activitySvc.ProcessInboundMediaTask(ctx, mediaCtx, task); err != nil {
		h.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Processing error encountered: %v", err))
		return
	}

	w.Header().Set(headerContentType, "application/json")
	w.WriteHeader(http.StatusAccepted)

	// Replaced memory allocation loop using zero-allocation byte block appends returning temporary object key
	responseBytes := fmt.Appendf(nil, `{"status":"committed","object_key":"%s"}`, tempObjectKey)
	_, _ = w.Write(responseBytes)
}

func (h *MediaUploadHandler) writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set(headerContentType, "application/json")
	w.WriteHeader(status)

	// Replaced memory allocation loop using zero-allocation byte block appends
	errorBytes := fmt.Appendf(nil, `{"error":%q}`, msg)
	_, _ = w.Write(errorBytes)
}
