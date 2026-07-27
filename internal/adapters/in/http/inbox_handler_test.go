package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gojuno/minimock/v3"
	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/port/portmock"
)

func TestInboxHandler_MethodNotAllowed(t *testing.T) {
	mc := minimock.NewController(t)

	storage := portmock.NewStoragePortMock(mc)
	handler := inhttp.NewInboxHandler(storage)

	req := httptest.NewRequest(http.MethodGet, "/inbox", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 MethodNotAllowed, got %d", rec.Code)
	}
}

func TestInboxHandler_Unauthenticated(t *testing.T) {
	mc := minimock.NewController(t)

	storage := portmock.NewStoragePortMock(mc)
	handler := inhttp.NewInboxHandler(storage)

	req := httptest.NewRequest(http.MethodPost, "/inbox", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized for context lacking credentials, got %d", rec.Code)
	}
}

func TestInboxHandler_Success(t *testing.T) {
	mc := minimock.NewController(t)

	storage := portmock.NewStoragePortMock(mc)
	storage.EnqueueInboundMock.Inspect(func(ctx context.Context, id, activityIRI, objectIRI, targetDomain string, payload []byte) {
		if activityIRI != "https://remote.com" {
			t.Errorf("Expected activityIRI to be 'https://remote.com', got %s", activityIRI)
		}
		if objectIRI != "https://remote.com" {
			t.Errorf("Expected objectIRI to be 'https://remote.com', got %s", objectIRI)
		}
		if targetDomain != "remote.com" {
			t.Errorf("Expected targetDomain to be 'remote.com', got %s", targetDomain)
		}
	}).Return(nil)

	handler := inhttp.NewInboxHandler(storage)

	payload := []byte(`{"id":"https://remote.com","type":"Create","object":{"id":"https://remote.com"}}`)
	req := httptest.NewRequest(http.MethodPost, "/inbox", bytes.NewReader(payload))
	req.Host = "remote.com"
	rec := httptest.NewRecorder()

	// Inject the pre-authenticated actor IRI into the request context, simulating the signature middleware pass
	ctx := context.WithValue(req.Context(), middleware.AuthenticatedActorKey, "https://remote.com")
	req = req.WithContext(ctx)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("Expected status 202 Accepted, got %d. Body: %s", rec.Code, rec.Body.String())
	}
}
