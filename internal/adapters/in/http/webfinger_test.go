package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	inhttp "sprezz/internal/adapters/in/http"
)

func TestHandleWebfinger_MissingResource(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger", nil)
	req.Host = "localhost"
	rec := httptest.NewRecorder()

	// Injecting allowed test domains via the new adapter function closure
	allowedDomains := []string{"localhost", "sprezz.net"}
	handler := inhttp.HandleWebfinger(allowedDomains)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestHandleWebfinger_Success(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource=acct:alice@sprezz.net", nil)
	req.Host = "sprezz.net" // Setting host explicitly to pass tenant lookup validation rules
	rec := httptest.NewRecorder()

	allowedDomains := []string{"sprezz.net"}
	handler := inhttp.HandleWebfinger(allowedDomains)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/jrd+json" {
		t.Errorf("Expected Content-Type application/jrd+json, got %s", contentType)
	}

	var resp inhttp.WebfingerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode Webfinger JSON response: %v", err)
	}

	if resp.Subject != "acct:alice@sprezz.net" {
		t.Errorf("Expected subject acct:alice@sprezz.net, got %s", resp.Subject)
	}

	if len(resp.Links) < 2 {
		t.Fatalf("Expected at least 2 links, got %d", len(resp.Links))
	}
}

func TestHandleWebfinger_ForbiddenDomain(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource=acct:alice@roguedomain.com", nil)
	req.Host = "roguedomain.com"
	rec := httptest.NewRecorder()

	allowedDomains := []string{"tenant-a.example", "tenant-b.example"}
	handler := inhttp.HandleWebfinger(allowedDomains)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 Forbidden for unconfigured domain, got %d", rec.Code)
	}
}

func TestHandleWebfinger_UsesConfiguredTenantDomains(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource=acct:alice@tenant-a.example", nil)
	req.Host = "tenant-a.example"
	rec := httptest.NewRecorder()

	// Simply pass the test domains in—no JSON files or disk IO operations needed!
	allowedDomains := []string{"tenant-a.example", "tenant-b.example"}
	handler := inhttp.HandleWebfinger(allowedDomains)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var resp inhttp.WebfingerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode Webfinger JSON response: %v", err)
	}

	if resp.Subject != "acct:alice@tenant-a.example" {
		t.Errorf("Expected subject acct:alice@tenant-a.example, got %s", resp.Subject)
	}
	if len(resp.Aliases) != 1 || resp.Aliases[0] != "https://tenant-a.example/actors/alice" {
		t.Errorf("Expected alias to use configured tenant domain, got %v", resp.Aliases)
	}
	if len(resp.Links) < 1 || resp.Links[0].Href != "https://tenant-a.example/actors/alice" {
		t.Errorf("Expected self link to use configured tenant domain, got %+v", resp.Links)
	}
}
