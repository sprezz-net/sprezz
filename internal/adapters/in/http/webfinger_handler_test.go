package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"
)

func TestHandleWebfinger_Success_ByHandle(t *testing.T) {
	mc := minimock.NewController(t)

	tenantDomains := []string{"sprezz.net"}
	username := "alice"
	actorUUID := "8f6c5b4a-2e1d-4c3b-9a8b-7f6e5d4c3b2a"
	actorIRI := "https://sprezz.net" + actorUUID

	mockStorage := portmock.NewStoragePortMock(mc)
	mockStorage.GetActorProfileFromGraphMock.Inspect(func(ctx context.Context, tenantID int32, u string) {
		if u != username {
			t.Errorf("Expected username %s, got %s", username, u)
		}
	}).Return(&model.ActorProfile{
		UUID:         actorUUID,
		IRI:          actorIRI,
		Username:     username,
		NomadGUID:    "nomad-guid-abc-123",
		PublicKeyPEM: "mock-public-key",
	}, nil)

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
	mc := minimock.NewController(t)

	tenantDomains := []string{"sprezz.net"}
	actorUUID := "9a8b7f6e-5d4c-3b2a-1a2b-3c4d5e6f7a8b"
	actorIRI := "https://sprezz.net" + actorUUID

	mockStorage := portmock.NewStoragePortMock(mc)
	mockStorage.GetActorProfileByIRIMock.Inspect(func(ctx context.Context, tenantID int32, iri string) {
		if iri != actorIRI {
			t.Errorf("Expected IRI %s, got %s", actorIRI, iri)
		}
	}).Return(&model.ActorProfile{
		UUID:         actorUUID,
		IRI:          actorIRI,
		Username:     "bob",
		PublicKeyPEM: "mock-public-key",
	}, nil)

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
	mc := minimock.NewController(t)

	tenantDomains := []string{"sprezz.net"}
	mockStorage := portmock.NewStoragePortMock(mc)

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
	mc := minimock.NewController(t)

	tenantDomains := []string{"sprezz.net"}
	mockStorage := portmock.NewStoragePortMock(mc)

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
