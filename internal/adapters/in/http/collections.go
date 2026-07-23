package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"sprezz/internal/domain/model"
)

const (
	internalServerError = "Internal server error"
	headerContentType   = "Content-Type"
)

type CollectionReader interface {
	GetLatestPayload(context.Context, string) ([]byte, error)
	GetCollectionPayloads(context.Context, string, string, int, int) ([][]byte, error)
	StreamQuadsBySubject(context.Context, string) ([]model.Quad, error)
}

type ActorHandler struct {
	storage CollectionReader
}

func NewActorHandler(storage CollectionReader) *ActorHandler {
	return &ActorHandler{storage: storage}
}

func (h *ActorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	actorIRI, collection, ok := actorRoute(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if collection == "" {
		h.serveActor(w, r, actorIRI)
		return
	}
	if collection == "followers" || collection == "following" {
		h.serveRelationshipCollection(w, r, actorIRI, collection)
		return
	}
	h.servePayloadCollection(w, r, actorIRI, collection)
}

func actorRoute(r *http.Request) (string, string, bool) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "actors" || parts[1] == "" {
		return "", "", false
	}
	if len(parts) == 3 && !validCollection(parts[2]) {
		return "", "", false
	}
	return "https://" + requestHost(r) + "/actors/" + parts[1], collectionPart(parts), true
}

func validCollection(collection string) bool {
	return collection == "inbox" || collection == "outbox" || collection == "followers" || collection == "following"
}

func collectionPart(parts []string) string {
	if len(parts) == 3 {
		return parts[2]
	}
	return ""
}

func (h *ActorHandler) serveActor(w http.ResponseWriter, r *http.Request, actorIRI string) {
	payload, err := h.storage.GetLatestPayload(r.Context(), actorIRI)
	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}
	if len(payload) == 0 {
		http.NotFound(w, r)
		return
	}
	writeActivityJSON(w, payload)
}

func (h *ActorHandler) serveRelationshipCollection(w http.ResponseWriter, r *http.Request, actorIRI, collection string) {
	quads, err := h.storage.StreamQuadsBySubject(r.Context(), actorIRI)
	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}
	predicate := "https://www.w3.org/ns/activitystreams#" + strings.TrimSuffix(collection, "s")
	items := make([]string, 0)
	for _, quad := range quads {
		if quad.Predicate == predicate && !quad.IsLiteral() {
			items = append(items, quad.Object)
		}
	}
	writeCollection(w, r.URL.String(), items)
}

func (h *ActorHandler) servePayloadCollection(w http.ResponseWriter, r *http.Request, actorIRI, collection string) {
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
	w.Header().Set(headerContentType, "application/ld+json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "OrderedCollection", "id": r.URL.String(), "orderedItems": items})
}

func writeCollection(w http.ResponseWriter, id string, items []string) {
	w.Header().Set(headerContentType, "application/ld+json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "OrderedCollection", "id": id, "totalItems": len(items), "orderedItems": items})
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

func writeActivityJSON(w http.ResponseWriter, payload []byte) {
	w.Header().Set(headerContentType, "application/activity+json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}
