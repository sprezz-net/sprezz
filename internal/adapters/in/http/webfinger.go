package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports"
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
func HandleWebfinger(tenantDomains []string, storage ports.StoragePort) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		resource := r.URL.Query().Get("resource")
		if resource == "" {
			http.Error(w, "Missing resource parameter", http.StatusBadRequest)
			return
		}

		tenantHost := RequestHost(r)
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

		w.Header().Set(headerContentType, "application/jrd+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(buildWebfingerResponse(resource, tenantHost, profile))
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
func resolveProfile(ctx context.Context, storage ports.StoragePort, resource, tenantHost string, tenantID int32) (*model.ActorProfile, error) {
	// If the resource is a direct URL pointer (e.g. https://yourdomain.com/actor/<uuidv4>)
	if strings.HasPrefix(resource, "https://") {
		if !strings.Contains(resource, tenantHost) {
			return nil, fmt.Errorf("resource domain mismatch for active tenant")
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

// buildWebfingerResponse isolates standard JRD document structural assembly matching Section 3.1 & 5.2.
func buildWebfingerResponse(resource, tenantHost string, profile *model.ActorProfile) WebfingerResponse {
	resp := WebfingerResponse{
		Subject:    resource,
		Aliases:    []string{profile.IRI}, // Outputs stable https://<domain>/actor/<uuidv4> location
		Properties: make(map[string]string),
		Links: []WebfingerReferenceLink{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: profile.IRI,
			},
		},
	}

	// Section 5.3: Add the Zot6/Nomad protocol pointer link using human-readable handle
	if profile.NomadGUID != "" {
		resp.Links = append(resp.Links, WebfingerReferenceLink{
			Rel:  "http://purl.org/zot/protocol/6.0#guid",
			Type: "application/x-zot+json",
			Href: fmt.Sprintf("https://%s/zot/channel/%s", tenantHost, profile.Username), // Uses username routing token
		})
	}

	return resp
}
