package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portstub"
)

// MockWebfingerStorageAdapter isolates graph reads from live SQL engines.
type MockWebfingerStorageAdapter struct {
	portstub.UnimplementedStoragePort
	OnGetActorProfileFromGraph func(ctx context.Context, tenantID int32, username string) (*model.ActorProfile, error)
	OnGetActorProfileByIRI     func(ctx context.Context, tenantID int32, iri string) (*model.ActorProfile, error)
}

func (m *MockWebfingerStorageAdapter) GetActorProfileFromGraph(ctx context.Context, tenantID int32, username string) (*model.ActorProfile, error) {
	if m.OnGetActorProfileFromGraph != nil {
		return m.OnGetActorProfileFromGraph(ctx, tenantID, username)
	}
	return nil, errors.New("not implemented")
}

func (m *MockWebfingerStorageAdapter) GetActorProfileByIRI(ctx context.Context, tenantID int32, iri string) (*model.ActorProfile, error) {
	if m.OnGetActorProfileByIRI != nil {
		return m.OnGetActorProfileByIRI(ctx, tenantID, iri)
	}
	return nil, errors.New("not implemented")
}

func TestHandleWebfinger_Success_ByHandle(t *testing.T) {
	tenantDomains := []string{"sprezz.net"}
	username := "alice"
	actorUUID := "8f6c5b4a-2e1d-4c3b-9a8b-7f6e5d4c3b2a"
	actorIRI := "https://sprezz.net" + actorUUID

	mockStorage := &MockWebfingerStorageAdapter{
		OnGetActorProfileFromGraph: func(ctx context.Context, tenantID int32, u string) (*model.ActorProfile, error) {
			if u != username {
				return nil, errors.New("not found")
			}
			return &model.ActorProfile{
				UUID:         actorUUID,
				IRI:          actorIRI,
				Username:     username,
				NomadGUID:    "nomad-guid-abc-123",
				PublicKeyPEM: "mock-public-key",
			}, nil
		},
	}

	handler := inhttp.HandleWebfinger(tenantDomains, mockStorage)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource=acct:alice@sprezz.net", nil)

	// Explicitly set the host parameter on the struct layout to satisfy RequestHost
	req.Host = "sprezz.net"

	ctx := context.WithValue(req.Context(), model.TenantIDKey, int32(1))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, actorIRI) {
		t.Errorf("Expected response to feature canonical Actor IRI link target %q", actorIRI)
	}

	expectedChannelHref := "https://sprezz.net"
	if !strings.Contains(body, expectedChannelHref) {
		t.Errorf("Expected Nomad protocol link reference channel to target %q", expectedChannelHref)
	}
}

func TestHandleWebfinger_Success_ByIRI(t *testing.T) {
	tenantDomains := []string{"sprezz.net"}
	actorUUID := "9a8b7f6e-5d4c-3b2a-1a2b-3c4d5e6f7a8b"
	actorIRI := "https://sprezz.net" + actorUUID

	mockStorage := &MockWebfingerStorageAdapter{
		OnGetActorProfileByIRI: func(ctx context.Context, tenantID int32, iri string) (*model.ActorProfile, error) {
			if iri != actorIRI {
				return nil, errors.New("not found")
			}
			return &model.ActorProfile{
				UUID:         actorUUID,
				IRI:          actorIRI,
				Username:     "bob",
				PublicKeyPEM: "mock-public-key",
			}, nil
		},
	}

	handler := inhttp.HandleWebfinger(tenantDomains, mockStorage)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource="+actorIRI, nil)

	// Explicitly set the host parameter on the struct layout to satisfy RequestHost
	req.Host = "sprezz.net"

	ctx := context.WithValue(req.Context(), model.TenantIDKey, int32(1))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, actorIRI) {
		t.Errorf("Expected response subject alias map to bind to canonical IRI path %q", actorIRI)
	}
}

func TestHandleWebfinger_DomainForbidden(t *testing.T) {
	tenantDomains := []string{"sprezz.net"}
	mockStorage := &MockWebfingerStorageAdapter{}

	handler := inhttp.HandleWebfinger(tenantDomains, mockStorage)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource=acct:alice@malicious.com", nil)

	req.Host = "unregistered-tenant.net"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403 (Forbidden) for unauthorized tenant domains, got %d", rr.Code)
	}
}

func TestHandleWebfinger_MalformedResource(t *testing.T) {
	tenantDomains := []string{"sprezz.net"}
	mockStorage := &MockWebfingerStorageAdapter{}

	handler := inhttp.HandleWebfinger(tenantDomains, mockStorage)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/webfinger?resource=acct:bad-string-format", nil)

	// Explicitly set the host parameter on the struct layout to satisfy RequestHost
	req.Host = "sprezz.net"

	ctx := context.WithValue(req.Context(), model.TenantIDKey, int32(1))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code 400 (Bad Request) for malformed account tokens, got %d", rr.Code)
	}
}
