package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"sprezz/internal/domain/ports"

	"github.com/google/uuid"
)

type InboxHandler struct {
	storage  ports.StoragePort
	verifier *SignatureVerifier
}

func NewInboxHandler(storage ports.StoragePort) *InboxHandler {
	return &InboxHandler{storage: storage}
}

func NewVerifiedInboxHandler(storage ports.StoragePort, verifier *SignatureVerifier) *InboxHandler {
	return &InboxHandler{storage: storage, verifier: verifier}
}

func (h *InboxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	host := RequestHost(r)
	body, err := readBody(r)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	if err := h.checkBlockedDomain(r.Context(), host); err != nil {
		writeInboxError(w, err)
		return
	}
	if err := h.verifyRequest(r, body); err != nil {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
	activityIRI, objectIRI, err := parseActivity(body)
	if err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if err := h.checkSenderDomain(r.Context(), body); err != nil {
		writeInboxError(w, err)
		return
	}
	if err := h.enqueueActivity(r, host, activityIRI, objectIRI, body); err != nil {
		http.Error(w, "Failed to queue activity", http.StatusInternalServerError)
		return
	}
	if err := h.recordInboxDelivery(r, host, activityIRI); err != nil {
		http.Error(w, "Failed to record inbox delivery", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

func readBody(r *http.Request) ([]byte, error) {
	// Fixed: Explicitly handle the error discard via an anonymous function closure to pass errcheck
	defer func() { _ = r.Body.Close() }()
	return io.ReadAll(io.LimitReader(r.Body, 10<<20))
}

func (h *InboxHandler) checkBlockedDomain(ctx context.Context, domain string) error {
	blocked, err := h.storage.IsDomainBlocked(ctx, domain)
	if err != nil {
		return err
	}
	if blocked {
		return errBlockedDomain
	}
	return nil
}

func (h *InboxHandler) verifyRequest(r *http.Request, body []byte) error {
	if h.verifier == nil {
		return nil
	}
	return h.verifier.Verify(r, body)
}

func parseActivity(body []byte) (string, string, error) {
	var rawActivity map[string]interface{}
	if err := json.Unmarshal(body, &rawActivity); err != nil {
		return "", "", err
	}
	activityIRI, _ := rawActivity["id"].(string)
	if activityIRI == "" {
		activityIRI = "urn:uuid:" + uuid.New().String()
	}
	return activityIRI, activityObjectIRI(rawActivity, activityIRI), nil
}

func activityObjectIRI(activity map[string]interface{}, fallback string) string {
	if object, ok := activity["object"].(map[string]interface{}); ok {
		if id, ok := object["id"].(string); ok {
			return id
		}
	}
	if object, ok := activity["object"].(string); ok {
		return object
	}
	return fallback
}

func (h *InboxHandler) checkSenderDomain(ctx context.Context, body []byte) error {
	var activity struct {
		Actor string `json:"actor"`
	}
	if err := json.Unmarshal(body, &activity); err != nil || activity.Actor == "" {
		return nil
	}
	parsed, err := url.Parse(activity.Actor)
	if err != nil || parsed.Hostname() == "" {
		return nil
	}
	return h.checkBlockedDomain(ctx, parsed.Hostname())
}

func (h *InboxHandler) enqueueActivity(r *http.Request, host, activityIRI, objectIRI string, body []byte) error {
	taskID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	return h.storage.EnqueueInbound(r.Context(), taskID.String(), activityIRI, objectIRI, host, body)
}

func (h *InboxHandler) recordInboxDelivery(r *http.Request, host, activityIRI string) error {
	actorIRI := inboxActorIRI(r, host)
	if actorIRI == "" {
		return nil
	}
	recorder, ok := h.storage.(interface {
		RecordActorInboxDelivery(context.Context, string, string) error
	})
	if ok {
		return recorder.RecordActorInboxDelivery(r.Context(), actorIRI, activityIRI)
	}
	return nil
}

func writeInboxError(w http.ResponseWriter, err error) {
	if errors.Is(err, errBlockedDomain) {
		http.Error(w, "Forbidden domain", http.StatusForbidden)
		return
	}
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

var errBlockedDomain = errors.New("blocked domain")

func inboxActorIRI(r *http.Request, host string) string {
	path := strings.TrimPrefix(r.URL.Path, "/inbox/")
	if path == r.URL.Path || path == "" || strings.Contains(path, "/") {
		return ""
	}
	return "https://" + host + "/actors/" + url.PathEscape(path)
}
