package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports"
)

type MockInboxStorage struct {
	BlockedDomain string
	Enqueued      bool
	RecordedIRI   string
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
func (m *MockInboxStorage) ClaimInboundBatch(ctx context.Context, b int) ([]model.InboundTask, error) {
	return nil, nil
}
func (m *MockInboxStorage) MarkInboundComplete(ctx context.Context, id string) error  { return nil }
func (m *MockInboxStorage) MarkInboundFailed(ctx context.Context, id, r string) error { return nil }
func (m *MockInboxStorage) GetNomadicIdentity(ctx context.Context, g string) (*model.NomadicIdentity, error) {
	return nil, nil
}
func (m *MockInboxStorage) UpsertNomadicIdentity(ctx context.Context, i *model.NomadicIdentity) error {
	return nil
}
func (m *MockInboxStorage) RegisterIdentityClone(ctx context.Context, g, h string, l bool) error {
	return nil
}
func (m *MockInboxStorage) GetActorPrivateKey(ctx context.Context, a string) (string, error) {
	return "", nil
}
func (m *MockInboxStorage) CreateGraphVersion(ctx context.Context, a, o string, p []byte) (int64, error) {
	return 0, nil
}
func (m *MockInboxStorage) SaveQuads(ctx context.Context, q []model.Quad) error      { return nil }
func (m *MockInboxStorage) SaveQuadIDs(ctx context.Context, q []model.QuadID) error  { return nil }
func (m *MockInboxStorage) RemoveQuadEdge(ctx context.Context, s, p, o string) error { return nil }
func (m *MockInboxStorage) GetLatestPayload(ctx context.Context, o string) ([]byte, error) {
	return nil, nil
}
func (m *MockInboxStorage) StreamQuadsBySubject(ctx context.Context, s string) ([]model.Quad, error) {
	return nil, nil
}
func (m *MockInboxStorage) GetCollectionPayloads(ctx context.Context, a, c string, l, o int) ([][]byte, error) {
	return nil, nil
}

func (m *MockInboxStorage) GetActorProfileFromGraph(ctx context.Context, tenantID int32, username string) (*model.ActorProfile, error) {
	return nil, nil
}
func (m *MockInboxStorage) GetActorProfileByIRI(ctx context.Context, tenantID int32, iri string) (*model.ActorProfile, error) {
	return nil, nil
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
