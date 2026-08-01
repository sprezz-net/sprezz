package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/pkg/httputil"
)

type WebfingerResponse struct {
	Subject    string                   `json:"subject"`
	Aliases    []string                 `json:"aliases,omitempty"`
	Properties map[string]string        `json:"properties,omitempty"`
	Links      []WebfingerReferenceLink `json:"links,omitempty"`
}

type WebfingerReferenceLink struct {
	Rel        string            `json:"rel"`
	Type       string            `json:"type,omitempty"`
	Href       string            `json:"href,omitempty"`
	Titles     map[string]string `json:"titles,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// HandleWebfinger takes the configured tenant domains and reads actors dynamically out of the RDF store.
func HandleWebfinger(tenantDomains []string, storage port.StoragePort) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		resource := r.URL.Query().Get("resource")
		if resource == "" {
			http.Error(w, "Missing resource parameter", http.StatusBadRequest)
			return
		}

		tenantHost := httputil.RequestHost(r)
		if !isTenantAllowed(tenantHost, tenantDomains) {
			http.Error(w, "Domain not in allowed tenants or host missing", http.StatusForbidden)
			return
		}

		tenantID, _ := ctx.Value(model.TenantIDKey).(int32)
		profile, err := resolveProfile(ctx, storage, resource, tenantHost, tenantID)
		if err != nil {
			// FIXED: Differentiate between a parsing syntax error (400) and a missing record error (404)
			if strings.Contains(err.Error(), "malformed") || strings.Contains(err.Error(), "mismatch") {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, "Actor resource profile not found in graph history", http.StatusNotFound)
			return
		}

		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJRDJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(buildWebfingerResponse(resource, profile))
	}
}

// isTenantAllowed isolates the loop matching logic to drop cognitive weight.
func isTenantAllowed(tenantHost string, tenantDomains []string) bool {
	if tenantHost == "" {
		return false
	}
	for _, domain := range tenantDomains {
		if strings.EqualFold(strings.TrimSpace(domain), tenantHost) {
			return true
		}
	}
	return false
}

// resolveProfile routes lookups either through direct stable IRIs or human-readable handles.
func resolveProfile(ctx context.Context, storage port.StoragePort, resource, tenantHost string, tenantID int32) (*model.ActorProfile, error) {
	// If the resource is a direct URL pointer (e.g. https://yourdomain.com/actor/<uuidv4>)
	if strings.HasPrefix(resource, "https://") || strings.HasPrefix(resource, "http://") {
		if !strings.Contains(resource, tenantHost) {
			return nil, fmt.Errorf("resource domain mismatch for active tenant")
		}
		cleanResource := strings.TrimRight(resource, "/")
		if cleanResource == "https://"+tenantHost || cleanResource == "http://"+tenantHost {
			actorIRI, err := storage.GetActorIRIByUsername(ctx, tenantID, "server")
			if err == nil && actorIRI != "" {
				resource = actorIRI
			}
		}
		profile, err := storage.GetActorProfileByIRI(ctx, tenantID, resource)
		if err != nil {
			return nil, fmt.Errorf("actor resource profile not found by IRI")
		}
		return profile, nil
	}

	// Fallback to standard human-readable acct: handles
	username, err := parseAndValidateResource(resource, tenantHost)
	if err != nil {
		return nil, err
	}

	// Dynamic System Actor UUIDv4 Profile Resolution Intercept
	if strings.EqualFold(username, "server") {
		// Look up only the canonical destination path string
		actorIRI, err := storage.GetActorIRIByUsername(ctx, tenantID, "server")
		if err == nil {
			// Return a flat, minimal profile
			return &model.ActorProfile{
				IRI:      actorIRI,
				Username: "server",
			}, nil
		}
	}

	profile, err := storage.GetActorProfileFromGraph(ctx, tenantID, username)
	if err != nil {
		return nil, fmt.Errorf("actor resource profile not found by handle")
	}
	return profile, nil
}

// parseAndValidateResource handles string manipulation bounds checking inside an isolated scope.
func parseAndValidateResource(resource, tenantHost string) (string, error) {
	cleanResource := strings.TrimPrefix(resource, "acct:")
	parts := strings.Split(cleanResource, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("malformed resource parameter format") // Must feature "malformed"
	}

	username := parts[0]
	resourceDomain := parts[1]

	if !strings.EqualFold(tenantHost, resourceDomain) {
		return "", fmt.Errorf("resource domain mismatch for active tenant") // Must feature "mismatch"
	}

	return username, nil
}

func buildWebfingerResponse(resource string, profile *model.ActorProfile) WebfingerResponse {
	resp := WebfingerResponse{
		Subject:    resource,
		Aliases:    []string{profile.IRI}, // Outputs the precise, safe UUIDv4 path string
		Properties: make(map[string]string),
		Links: []WebfingerReferenceLink{
			{
				Rel:  "self",
				Type: httputil.ContentTypeActivityJSON,
				Href: profile.IRI, // Points remote instances strictly to the Actor Profile
			},
		},
	}

	// Prevent system machine accounts from leaking human Nomadic parameters.
	if strings.EqualFold(profile.Username, "server") {
		return resp
	}

	// Existing human channel Nomadic fallback linkages.
	if profile.NomadGUID != "" {
		resp.Links = append(resp.Links, WebfingerReferenceLink{
			Rel:  model.PredicateNomadGUID,
			Type: "application/x-zot+json",
			Href: profile.NomadGUID,
		})
	}

	return resp
}
