package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	inhttp "sprezz/internal/adapters/in/http"
)

func TestHandleWebfinger_MissingResource(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger", nil)
	rec := httptest.NewRecorder()

	inhttp.HandleWebfinger(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestHandleWebfinger_Success(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource=acct:alice@sprezz.net", nil)
	rec := httptest.NewRecorder()

	inhttp.HandleWebfinger(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
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

func TestHandleWebfingerWithConfig_UsesConfiguredTenantDomains(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "tenants.json")
	cfg := []byte(`{"domains":["tenant-a.example","tenant-b.example"]}`)
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource=acct:alice@tenant-a.example", nil)
	req.Host = "tenant-a.example"
	rec := httptest.NewRecorder()

	inhttp.HandleWebfingerWithConfig(rec, req, cfgPath)

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
