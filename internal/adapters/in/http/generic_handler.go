package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/pkg/httputil"

	"github.com/google/uuid"
)

const (
	internalServerError = "Internal server error"
)

type GenericHandler struct {
	storage port.StoragePort
}

func NewGenericHandler(storage port.StoragePort) *GenericHandler {
	return &GenericHandler{storage: storage}
}

func writeActivityJSON(w http.ResponseWriter, payload []byte) {
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeActivityJSON)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func collectionPage(r *http.Request) (int, int) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func writeCollection(w http.ResponseWriter, id string, items []string) {
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeLDJSON)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "OrderedCollection", "id": id, "totalItems": len(items), "orderedItems": items})
}

func (h *GenericHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestedIRI := httputil.HTTPSPrefix + httputil.RequestHost(r) + r.URL.Path

	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r, requestedIRI)
	case http.MethodPost:
		h.handlePost(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func extractCollection(requestedIRI string) (string, string) {
	suffixes := []string{"/inbox", "/outbox", "/followers", "/following", "/likes", "/shares", "/replies", "/contextHistory", "/context"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(requestedIRI, suffix) {
			return strings.TrimPrefix(suffix, "/"), strings.TrimSuffix(requestedIRI, suffix)
		}
	}
	return "", requestedIRI
}

func (h *GenericHandler) handleServerActorRedirect(w http.ResponseWriter, r *http.Request, actorIRI, collection string) bool {
	ctx := r.Context()
	tenantHost := httputil.RequestHost(r)
	cleanActorIRI := strings.TrimRight(actorIRI, "/")
	if cleanActorIRI != httputil.HTTPSPrefix+tenantHost && cleanActorIRI != httputil.HTTPPrefix+tenantHost {
		return false
	}

	tenantID, _ := ctx.Value(model.TenantIDKey).(int32)
	if tenantID == 0 {
		return false
	}

	serverIRI, err := h.storage.GetActorIRIByUsername(ctx, tenantID, "server")
	if err == nil && serverIRI != "" {
		targetRedirect := serverIRI
		if collection != "" {
			targetRedirect = serverIRI + "/" + collection
		}
		http.Redirect(w, r, targetRedirect, http.StatusSeeOther)
		return true
	}
	return false
}

func (h *GenericHandler) handleAliasRedirect(w http.ResponseWriter, r *http.Request, actorIRI, collection string) {
	canonicalIRI, err := h.storage.GetActorIRIByAlias(r.Context(), actorIRI)
	if err == nil && canonicalIRI != "" {
		targetRedirect := canonicalIRI
		if collection != "" {
			targetRedirect = canonicalIRI + "/" + collection
		}
		http.Redirect(w, r, targetRedirect, http.StatusSeeOther)
		return
	}
	http.NotFound(w, r)
}

func handleHTMLRedirect(w http.ResponseWriter, r *http.Request, payload []byte) bool {
	accept := r.Header.Get(httputil.HeaderAccept)
	if !strings.Contains(accept, "text/html") {
		return false
	}
	var profile struct {
		PreferredUsername string `json:"preferredUsername"`
	}
	if err := json.Unmarshal(payload, &profile); err == nil && profile.PreferredUsername != "" {
		http.Redirect(w, r, httputil.HTTPSPrefix+httputil.RequestHost(r)+"/@"+profile.PreferredUsername, http.StatusFound)
		return true
	}
	return false
}

func (h *GenericHandler) handleGet(w http.ResponseWriter, r *http.Request, requestedIRI string) {
	ctx := r.Context()

	// 1. Detect and strip standard collections from the requested path
	collection, actorIRI := extractCollection(requestedIRI)

	// Intercept FEP-d556 root domain and decoupled shared inbox lookups
	if h.handleServerActorRedirect(w, r, actorIRI, collection) {
		return
	}

	// 2. Lookup the actor payload directly by IRI
	payload, err := h.storage.GetLatestPayload(ctx, actorIRI)
	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}

	// 3. If not found directly, check if the requested IRI is a registered alsoKnownAs alias
	if len(payload) == 0 {
		h.handleAliasRedirect(w, r, actorIRI, collection)
		return
	}

	// 4. Content Negotiation / MIME Type Branching
	if handleHTMLRedirect(w, r, payload) {
		return
	}

	// 5. Serve the actual ActivityPub payload collections or actor profile
	if collection == "" {
		writeActivityJSON(w, payload)
		return
	}
	if collection == "followers" || collection == "following" {
		h.serveRelationshipCollection(w, r, actorIRI, collection)
		return
	}
	if collection == "likes" || collection == "shares" || collection == "replies" || collection == "context" || collection == "contextHistory" {
		h.serveEngagementCollection(w, r, actorIRI, collection)
		return
	}
	h.servePayloadCollection(w, r, actorIRI, collection)
}

func (h *GenericHandler) serveEngagementCollection(w http.ResponseWriter, r *http.Request, objectIRI, collection string) {
	var items []string
	var err error

	switch collection {
	case "likes":
		items, err = h.storage.GetLikesForObject(r.Context(), objectIRI)
	case "shares":
		items, err = h.storage.GetSharesForObject(r.Context(), objectIRI)
	case "replies":
		items, err = h.storage.GetRepliesForObject(r.Context(), objectIRI)
	case "context":
		contextIRI := objectIRI + "/context"
		items, err = h.storage.GetObjectsByContext(r.Context(), contextIRI)
	case "contextHistory":
		contextHistoryIRI := objectIRI + "/contextHistory"
		items, err = h.storage.GetObjectsByContext(r.Context(), contextHistoryIRI)
	default:
		http.Error(w, "Unsupported collection", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}

	writeCollection(w, r.URL.String(), items)
}

func (h *GenericHandler) serveRelationshipCollection(w http.ResponseWriter, r *http.Request, actorIRI, collection string) {
	quads, err := h.storage.StreamQuadsBySubject(r.Context(), actorIRI)
	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}

	predicate := model.NamespaceActivityStreams + collection

	items := make([]string, 0)
	for _, quad := range quads {
		if quad.Predicate == predicate && !quad.IsLiteral() {
			items = append(items, quad.Object)
		}
	}
	writeCollection(w, r.URL.String(), items)
}

func (h *GenericHandler) servePayloadCollection(w http.ResponseWriter, r *http.Request, actorIRI, collection string) {
	limit, offset := collectionPage(r)
	payloads, err := h.storage.GetCollectionPayloads(r.Context(), actorIRI, collection, limit, offset)
	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}
	items := make([]json.RawMessage, 0, len(payloads))
	for _, payload := range payloads {
		items = append(items, json.RawMessage(payload))
	}
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeLDJSON)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "OrderedCollection", "id": r.URL.String(), "orderedItems": items})
}

func (h *GenericHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract the pre-authenticated actor IRI straight from the middleware context
	authenticatedActor := middleware.GetAuthenticatedActor(ctx)
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

	taskID, err := uuid.NewV7()
	if err != nil {
		http.Error(w, "Internal Server Error: Unable to generate task ID", http.StatusInternalServerError)
		return
	}

	tenantID, _ := ctx.Value(model.TenantIDKey).(int32)
	if tenantID == 0 {
		http.Error(w, "Internal Server Error: Unknown tenant context", http.StatusInternalServerError)
		return
	}

	// Purely enqueue the inbound activity. Direct vs Shared delivery resolution is fully
	// offloaded to the async background worker ProcessInboundTask.
	err = h.storage.EnqueueInbound(ctx, taskID.String(), activity.ID, activity.Object.ID, tenantID, body)
	if err != nil {
		http.Error(w, "Internal Server Error: Ingestion queue failure", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
