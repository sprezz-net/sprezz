package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/model"
)

type mockSignatureVerifier struct {
	OnVerify func(r *http.Request, body []byte) error
}

func (m *mockSignatureVerifier) Verify(r *http.Request, body []byte) error {
	if m.OnVerify != nil {
		return m.OnVerify(r, body)
	}
	return errors.New("signature mismatch")
}

type mockStorageStub struct{}

func (m *mockStorageStub) IsDomainBlocked(ctx context.Context, d string) (bool, error) {
	return d == "https://malicious.com", nil
}
func (m *mockStorageStub) EnqueueInbound(ctx context.Context, id, a, o, t string, p []byte) error { return nil }
func (m *mockStorageStub) ClaimInboundBatch(ctx context.Context, b int) ([]model.InboundTask, error) { return nil, nil }
func (m *mockStorageStub) MarkInboundComplete(ctx context.Context, id string) error  { return nil }
func (m *mockStorageStub) MarkInboundFailed(ctx context.Context, id, r string) error { return nil }
func (m *mockStorageStub) GetNomadicIdentity(ctx context.Context, g string) (*model.NomadicIdentity, error) { return nil, nil }
func (m *mockStorageStub) UpsertNomadicIdentity(ctx context.Context, i *model.NomadicIdentity) error { return nil }
func (m *mockStorageStub) RegisterIdentityClone(ctx context.Context, g, h string, l bool) error { return nil }
func (m *mockStorageStub) GetActorPrivateKey(ctx context.Context, a string) (string, error) { return "", nil }
func (m *mockStorageStub) CreateGraphVersion(ctx context.Context, a, o string, p []byte) (int64, error) { return 0, nil }
func (m *mockStorageStub) SaveQuads(ctx context.Context, q []model.Quad) error      { return nil }
func (m *mockStorageStub) SaveQuadIDs(ctx context.Context, q []model.QuadID) error { return nil }
func (m *mockStorageStub) RemoveQuadEdge(ctx context.Context, s, p, o string) error { return nil }
func (m *mockStorageStub) GetLatestPayload(ctx context.Context, o string) ([]byte, error) { return nil, nil }
func (m *mockStorageStub) StreamQuadsBySubject(ctx context.Context, s string) ([]model.Quad, error) { return nil, nil }
func (m *mockStorageStub) GetCollectionPayloads(ctx context.Context, a, c string, l, o int) ([][]byte, error) { return nil, nil }
func (m *mockStorageStub) RecordActorInboxDelivery(ctx context.Context, actorIRI, activityIRI string) error { return nil }

func TestSignatureValidator_Handler(t *testing.T) {
	storage := &mockStorageStub{}
	verifier := &mockSignatureVerifier{
		OnVerify: func(r *http.Request, body []byte) error {
			sig := r.Header.Get("Signature")
			if sig == "valid-sig" {
				r.Header.Set("X-Actor-IRI", "https://remote-actor.com")
				return nil
			}
			if sig == "blocked-sig" {
				r.Header.Set("X-Actor-IRI", "https://malicious.com")
				return nil
			}
			return errors.New("invalid signature parsing matrix")
		},
	}

	validator := middleware.NewSignatureValidator(verifier, storage)
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor := middleware.GetAuthenticatedActor(r.Context())
		w.Header().Set("X-Authenticated-Actor", actor)
		w.WriteHeader(http.StatusAccepted)
	})

	wrappedHandler := validator.Handler(nextHandler)

	tests := []struct {
		name           string
		method         string
		signature      string
		expectedStatus int
		expectedActor  string
	}{
		{
			name:           "Valid Authenticated Request Verification Pass",
			method:         http.MethodPost,
			signature:      "valid-sig",
			expectedStatus: http.StatusAccepted,
			expectedActor:  "https://remote-actor.com",
		},
		{
			name:           "Rejected Unauthorized Cryptographic Forgery",
			method:         http.MethodPost,
			signature:      "forged-sig",
			expectedStatus: http.StatusUnauthorized,
			expectedActor:  "",
		},
		{
			name:           "Rejected Forbidden Blocked Actor Match",
			method:         http.MethodPost,
			signature:      "blocked-sig",
			expectedStatus: http.StatusForbidden,
			expectedActor:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "https://sprezz.net", nil)
			if tt.signature != "" {
				req.Header.Set("Signature", tt.signature)
			}
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Signature validation handling failure for %q: got status %d, want %d", tt.name, rr.Code, tt.expectedStatus)
			}
		})
	}
}
