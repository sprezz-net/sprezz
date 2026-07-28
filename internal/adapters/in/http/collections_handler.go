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
	GetActorIRIByAlias(context.Context, string) (string, error)
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

	ctx := r.Context()
	requestedIRI := "https://" + RequestHost(r) + r.URL.Path

	// 1. Detect and strip standard collections from the requested path
	collection := ""
	actorIRI := requestedIRI

	if strings.HasSuffix(requestedIRI, "/inbox") {
		collection = "inbox"
		actorIRI = strings.TrimSuffix(requestedIRI, "/inbox")
	} else if strings.HasSuffix(requestedIRI, "/outbox") {
		collection = "outbox"
		actorIRI = strings.TrimSuffix(requestedIRI, "/outbox")
	} else if strings.HasSuffix(requestedIRI, "/followers") {
		collection = "followers"
		actorIRI = strings.TrimSuffix(requestedIRI, "/followers")
	} else if strings.HasSuffix(requestedIRI, "/following") {
		collection = "following"
		actorIRI = strings.TrimSuffix(requestedIRI, "/following")
	}

	// 2. Lookup the actor payload directly by IRI
	payload, err := h.storage.GetLatestPayload(ctx, actorIRI)
	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}

	// 3. If not found directly, check if the requested IRI is a registered alsoKnownAs alias
	if len(payload) == 0 {
		canonicalIRI, err := h.storage.GetActorIRIByAlias(ctx, actorIRI)
		if err == nil && canonicalIRI != "" {
			// Found alias! Perform HTTP 303 See Other redirect to canonical profile (with collection if applicable)
			targetRedirect := canonicalIRI
			if collection != "" {
				targetRedirect = canonicalIRI + "/" + collection
			}
			http.Redirect(w, r, targetRedirect, http.StatusSeeOther)
			return
		}

		http.NotFound(w, r)
		return
	}

	// 4. Content Negotiation / MIME Type Branching
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") {
		var profile struct {
			PreferredUsername string `json:"preferredUsername"`
		}
		if err := json.Unmarshal(payload, &profile); err == nil && profile.PreferredUsername != "" {
			// Browser client: HTTP 302 redirect to frontend Web UI vanity profile presentation layer
			http.Redirect(w, r, "https://"+RequestHost(r)+"/@"+profile.PreferredUsername, http.StatusFound)
			return
		}
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
	h.servePayloadCollection(w, r, actorIRI, collection)
}

func (h *ActorHandler) serveRelationshipCollection(w http.ResponseWriter, r *http.Request, actorIRI, collection string) {
	quads, err := h.storage.StreamQuadsBySubject(r.Context(), actorIRI)
	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}

	// Keep plural forms to respect ActivityPub vocabulary specifications
	predicate := "https://www.w3.org/ns/activitystreams#" + collection

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

func writeActivityJSON(w http.ResponseWriter, payload []byte) {
	w.Header().Set(headerContentType, "application/activity+json")
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
	w.Header().Set(headerContentType, "application/ld+json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "OrderedCollection", "id": id, "totalItems": len(items), "orderedItems": items})
}
