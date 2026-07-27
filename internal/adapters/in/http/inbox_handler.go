package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/port"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type InboxHandler struct {
	storage port.StoragePort
}

// NewInboxHandler initializes a clean, decoupled handler. Verification is handled at the edge routing middleware.
func NewInboxHandler(storage port.StoragePort) *InboxHandler {
	return &InboxHandler{storage: storage}
}

func (h *InboxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Extract the pre-authenticated actor IRI straight from the middleware context
	authenticatedActor := middleware.GetAuthenticatedActor(r.Context())
	if authenticatedActor == "" {
		http.Error(w, "Unauthorized: Request context lacks verified signature validation", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad Request: Unable to read payload", http.StatusBadRequest)
		return
	}

	var activity struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Object struct {
			ID string `json:"id"`
		} `json:"object"`
	}

	if err := json.Unmarshal(body, &activity); err != nil {
		http.Error(w, "Bad Request: Malformed JSON activity payload", http.StatusBadRequest)
		return
	}

	// 2. Queue the task for the asynchronous background processing workers
	taskID, err := uuid.NewV7() // Generates standard sequential UUIDv7 tokens
	if err != nil {
		http.Error(w, "Internal Server Error: Unable to generate task ID", http.StatusInternalServerError)
		return
	}

	targetDomain := r.Host
	if idx := strings.IndexByte(targetDomain, ':'); idx != -1 {
		targetDomain = targetDomain[:idx]
	}

	err = h.storage.EnqueueInbound(r.Context(), taskID.String(), activity.ID, activity.Object.ID, targetDomain, body)
	if err != nil {
		http.Error(w, "Internal Server Error: Ingestion queue failure", http.StatusInternalServerError)
		return
	}

	// 3. If targeted to a specific user inbox route (e.g., /inbox/{actor}), record delivery tracking metrics
	if actorParam := chi.URLParam(r, "actor"); actorParam != "" {
		_ = h.storage.RecordActorInboxDelivery(r.Context(), authenticatedActor, activity.ID)
	}

	w.WriteHeader(http.StatusAccepted)
}
