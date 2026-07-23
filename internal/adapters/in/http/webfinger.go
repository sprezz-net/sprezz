package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"sprezz/internal/config"
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

func HandleWebfinger(w http.ResponseWriter, r *http.Request) {
	HandleWebfingerWithConfig(w, r, config.DefaultTenantConfigPath())
}

func HandleWebfingerWithConfig(w http.ResponseWriter, r *http.Request, tenantConfigPath string) {
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		http.Error(w, "Missing resource parameter", http.StatusBadRequest)
		return
	}

	tenantHost := requestHost(r)
	if tenantHost == "" {
		tenantHost = strings.TrimSuffix(resource, "")
	}
	base := "https://" + tenantHost

	cfg, err := config.LoadTenantConfig(tenantConfigPath)
	if err == nil && !cfg.Contains(tenantHost) {
		http.Error(w, fmt.Sprintf("Tenant %q not configured", tenantHost), http.StatusNotFound)
		return
	}
	if err != nil {
		cfg = nil
	}

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
				Rel:  "http://purl.org/zot/protocol",
				Type: "application/x-zot+json",
				Href: base + "/zot/channel/alice-guid-12345",
			},
		},
	}

	if err != nil {
		resp.Links = resp.Links[:1]
	}

	w.Header().Set("Content-Type", "application/jrd+json")
	_ = json.NewEncoder(w).Encode(resp)
}
