package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/ports"
	"sprezz/internal/domain/ports/portstest"
)

type MockInboxStorage struct {
	portstest.UnimplementedStoragePort // Composite fallback embedded stub (de-bloating layout)
	BlockedDomain                      string
	Enqueued                           bool
	RecordedIRI                        string
}

var _ ports.StoragePort = (*MockInboxStorage)(nil)

func (m *MockInboxStorage) IsDomainBlocked(ctx context.Context, domainName string) (bool, error) {
	return domainName == m.BlockedDomain, nil
}

func (m *MockInboxStorage) EnqueueInbound(ctx context.Context, id, activityIRI, objectIRI, targetDomain string, payload []byte) error {
	m.Enqueued = true
	return nil
}

func (m *MockInboxStorage) RecordActorInboxDelivery(ctx context.Context, actorIRI, activityIRI string) error {
	m.RecordedIRI = actorIRI
	return nil
}

func TestInboxHandler_MethodNotAllowed(t *testing.T) {
	storage := &MockInboxStorage{}
	handler := inhttp.NewInboxHandler(storage)

	req := httptest.NewRequest(http.MethodGet, "/inbox", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 MethodNotAllowed, got %d", rec.Code)
	}
}

func TestInboxHandler_Unauthenticated(t *testing.T) {
	storage := &MockInboxStorage{}
	handler := inhttp.NewInboxHandler(storage)

	req := httptest.NewRequest(http.MethodPost, "/inbox", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized for context lacking credentials, got %d", rec.Code)
	}
}

func TestInboxHandler_Success(t *testing.T) {
	storage := &MockInboxStorage{}
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
	if !storage.Enqueued {
		t.Error("Expected activity to be enqueued in storage port")
	}
}
