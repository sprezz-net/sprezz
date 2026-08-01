package http

import (
	"context"
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
	service port.ActivityServicePort
}

func NewGenericHandler(storage port.StoragePort, service port.ActivityServicePort) *GenericHandler {
	return &GenericHandler{storage: storage, service: service}
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
	idx := strings.LastIndex(requestedIRI, "/")
	if idx == -1 {
		return "", requestedIRI
	}
	lastSegment := requestedIRI[idx+1:]
	if model.IsCollection(lastSegment) {
		return lastSegment, requestedIRI[:idx]
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
	if model.IsPrivateCollection(collection) {
		authenticatedActor := middleware.GetAuthenticatedActor(ctx)
		if authenticatedActor == "" || authenticatedActor != actorIRI {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}
	if collection == model.ShortFollowers || collection == model.ShortFollowing {
		h.serveRelationshipCollection(w, r, actorIRI, collection)
		return
	}
	if collection == model.ShortFollowersSync {
		h.serveFollowersSyncCollection(w, r, actorIRI)
		return
	}
	if collection == model.ShortLikes || collection == model.ShortShares || collection == model.ShortReplies || collection == model.ShortContext || collection == model.ShortContextHistory {
		h.serveEngagementCollection(w, r, actorIRI, collection)
		return
	}
	h.servePayloadCollection(w, r, actorIRI, collection)
}

func (h *GenericHandler) serveFollowersSyncCollection(w http.ResponseWriter, r *http.Request, actorIRI string) {
	ctx := r.Context()
	authenticatedActor := middleware.GetAuthenticatedActor(ctx)
	if authenticatedActor == "" {
		http.Error(w, "Unauthorized: Request context lacks verified signature validation", http.StatusUnauthorized)
		return
	}

	requesterDomain := extractDomain(authenticatedActor)
	if requesterDomain == "" {
		http.Error(w, "Bad Request: Invalid authenticated actor domain", http.StatusBadRequest)
		return
	}

	quads, err := h.storage.StreamQuadsBySubject(ctx, actorIRI)
	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}

	predicate := model.PredicateFollower
	items := make([]string, 0)
	for _, quad := range quads {
		if quad.Predicate == predicate && !quad.IsLiteral() {
			followerIRI := quad.Object
			if extractDomain(followerIRI) == requesterDomain {
				items = append(items, followerIRI)
			}
		}
	}

	writeCollection(w, r.URL.String(), items)
}

func (h *GenericHandler) serveEngagementCollection(w http.ResponseWriter, r *http.Request, objectIRI, collection string) {
	var items []string
	var err error

	switch collection {
	case model.ShortLikes:
		items, err = h.storage.GetLikesForObject(r.Context(), objectIRI)
	case model.ShortShares:
		items, err = h.storage.GetSharesForObject(r.Context(), objectIRI)
	case model.ShortReplies:
		items, err = h.storage.GetRepliesForObject(r.Context(), objectIRI)
	case model.ShortContext:
		contextIRI := objectIRI + "/" + model.ShortContext
		items, err = h.storage.GetObjectsByContext(r.Context(), contextIRI)
	case model.ShortContextHistory:
		contextHistoryIRI := objectIRI + "/" + model.ShortContextHistory
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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "OrderedCollection", "id": r.URL.String(), "totalItems": len(items), "orderedItems": items})
}

func parseCollectionSyncHeader(headerVal string) (collectionID, syncURL, digest string) {
	params := make(map[string]string)
	parts := strings.Split(headerVal, ",")
	for _, part := range parts {
		subParts := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(subParts) == 2 {
			k := strings.TrimSpace(subParts[0])
			v := strings.Trim(strings.TrimSpace(subParts[1]), "\"`")
			params[k] = v
		}
	}
	return params["collectionId"], params["url"], params["digest"]
}

func (h *GenericHandler) hasLocalRecipient(ctx context.Context, body []byte) bool {
	var envelope struct {
		To       interface{} `json:"to"`
		Cc       interface{} `json:"cc"`
		Audience interface{} `json:"audience"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}

	targets := make(map[string]struct{})
	collect := func(val interface{}) {
		switch v := val.(type) {
		case string:
			targets[v] = struct{}{}
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					targets[s] = struct{}{}
				}
			}
		}
	}
	collect(envelope.To)
	collect(envelope.Cc)
	collect(envelope.Audience)

	for target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		_, err := h.storage.GetActorDualKeys(ctx, target)
		if err == nil {
			return true
		}
	}
	return false
}

func (h *GenericHandler) triggerFollowersSync(r *http.Request, authenticatedActor string) {
	syncHeader := r.Header.Get("Collection-Synchronization")
	if syncHeader == "" || h.service == nil {
		return
	}
	collectionID, syncURL, digest := parseCollectionSyncHeader(syncHeader)
	if collectionID == "" || syncURL == "" || digest == "" {
		return
	}
	go func() {
		// Security check: only sync if the collection ID matches the authenticated actor's followers
		if collectionID == authenticatedActor+"/followers" {
			_ = h.service.SyncFollowers(context.Background(), authenticatedActor, collectionID, syncURL, digest)
		}
	}()
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

	if r.URL.Path == "/"+model.ShortInbox {
		if !h.hasLocalRecipient(ctx, body) {
			http.Error(w, "Bad Request: No valid local actor addressed in shared inbox delivery", http.StatusBadRequest)
			return
		}
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

	// Process FEP-8fcf Followers Collection synchronization asynchronously
	h.triggerFollowersSync(r, authenticatedActor)

	w.WriteHeader(http.StatusAccepted)
}
