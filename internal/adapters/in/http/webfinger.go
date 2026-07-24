package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type WebfingerResponse struct {
	Subject string                   `json:"subject"`
	Aliases []string                 `json:"aliases,omitempty"`
	Links   []WebfingerReferenceLink `json:"links"`
}

type WebfingerReferenceLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}

// HandleWebfinger takes the configured tenant domains directly as a dependency
func HandleWebfinger(tenantDomains []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resource := r.URL.Query().Get("resource")
		if resource == "" {
			http.Error(w, "Missing resource parameter", http.StatusBadRequest)
			return
		}

		tenantHost := RequestHost(r)
		if tenantHost == "" {
			http.Error(w, "Unable to determine host from request", http.StatusBadRequest)
			return
		}

		// Validate against configured domains passed from main.go
		found := false
		for _, domain := range tenantDomains {
			if strings.EqualFold(strings.TrimSpace(domain), tenantHost) {
				found = true
				break
			}
		}

		if !found {
			http.Error(w, fmt.Sprintf("Domain %q not in allowed tenants", tenantHost), http.StatusForbidden)
			return
		}

		base := "https://" + tenantHost
		actorIRI := base + "/actors/alice"

		resp := WebfingerResponse{
			Subject: resource,
			Aliases: []string{actorIRI},
			Links: []WebfingerReferenceLink{
				{
					Rel:  "self",
					Type: "application/activity+json",
					Href: actorIRI,
				},
				{
					Rel:  "http://purl.org",
					Type: "application/x-zot+json",
					Href: base + "/zot/channel/alice-guid-12345",
				},
			},
		}

		w.Header().Set("Content-Type", "application/jrd+json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
