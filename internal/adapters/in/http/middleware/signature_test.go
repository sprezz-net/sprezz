package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/ports"
	"sprezz/internal/domain/ports/portstest"
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

// MockStorageAdapter implements ports.StoragePort for middleware isolation testing.
type mockStorageStub struct {
	portstest.UnimplementedStoragePort // Composite fallback embedded stub (de-bloating layout)
}

var _ ports.StoragePort = (*mockStorageStub)(nil)

// Override only the blocklist method called by the validator middleware
func (m *mockStorageStub) IsDomainBlocked(ctx context.Context, d string) (bool, error) {
	return d == "https://malicious.com", nil
}

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
		{
			name:           "Bypass Evaluation For Non Mutation Requests",
			method:         http.MethodGet,
			signature:      "",
			expectedStatus: http.StatusAccepted,
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

			// Handled response validation loops via a distinct single-responsibility helper block
			assertResponse(t, tt.name, rr, tt.method, tt.expectedStatus, tt.expectedActor)
		})
	}
}

// assertResponse decouples validation loops from the test runner loop context to achieve absolute flat logic profiles.
func assertResponse(t *testing.T, name string, rr *httptest.ResponseRecorder, method string, expectedStatus int, expectedActor string) {
	if rr.Code != expectedStatus {
		t.Errorf("Signature validation handling failure for %q: got status %d, want %d", name, rr.Code, expectedStatus)
	}

	if expectedStatus == http.StatusAccepted && method == http.MethodPost {
		extractedActor := rr.Header().Get("X-Authenticated-Actor")
		if extractedActor != expectedActor {
			t.Errorf("Context propagation mismatch in %q: got %q, want %q", name, extractedActor, expectedActor)
		}
	}
}
