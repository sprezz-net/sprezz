package middleware_test

import (
	"sprezz/internal/pkg/httputil"

	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gojuno/minimock/v3"
	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/port/portmock"
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

func TestSignatureValidator_Handler(t *testing.T) {
	mc := minimock.NewController(t)

	storage := portmock.NewStoragePortMock(mc)
	storage.IsDomainBlockedMock.Set(func(ctx context.Context, d string) (bool, error) {
		return d == "https://malicious.com", nil
	})

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
		path           string
		contentType    string
		signature      string
		expectedStatus int
		expectedActor  string
	}{
		{
			name:           "Valid Authenticated Request Verification Pass",
			method:         http.MethodPost,
			path:           "https://sprezz.net/inbox",
			contentType:    "application/activity+json",
			signature:      "valid-sig",
			expectedStatus: http.StatusAccepted,
			expectedActor:  "https://remote-actor.com",
		},
		{
			name:           "Rejected Unauthorized Cryptographic Forgery",
			method:         http.MethodPost,
			path:           "https://sprezz.net/inbox",
			contentType:    "application/activity+json",
			signature:      "forged-sig",
			expectedStatus: http.StatusUnauthorized,
			expectedActor:  "",
		},
		{
			name:           "Rejected Forbidden Blocked Actor Match",
			method:         http.MethodPost,
			path:           "https://sprezz.net/inbox",
			contentType:    "application/activity+json",
			signature:      "blocked-sig",
			expectedStatus: http.StatusForbidden,
			expectedActor:  "",
		},
		{
			name:           "Bypass POST on Non-ActivityPub Content-Type",
			method:         http.MethodPost,
			path:           "https://sprezz.net/upload",
			contentType:    "multipart/form-data",
			signature:      "",
			expectedStatus: http.StatusAccepted,
			expectedActor:  "",
		},
		{
			name:           "Reject POST with Missing Content-Type Header",
			method:         http.MethodPost,
			path:           "https://sprezz.net/inbox",
			contentType:    "",
			signature:      "",
			expectedStatus: http.StatusBadRequest,
			expectedActor:  "",
		},
		{
			name:           "Reject POST to inbox with Non-ActivityPub Content-Type",
			method:         http.MethodPost,
			path:           "https://sprezz.net/inbox",
			contentType:    "text/plain",
			signature:      "",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedActor:  "",
		},
		{
			name:           "Reject POST to outbox with Non-ActivityPub Content-Type",
			method:         http.MethodPost,
			path:           "https://sprezz.net/outbox",
			contentType:    httputil.ContentTypeJSON,
			signature:      "",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedActor:  "",
		},
		{
			name:           "Bypass GET on Non-ActivityPub Content-Type with Signature",
			method:         http.MethodGet,
			path:           "https://sprezz.net/actor",
			contentType:    "text/html",
			signature:      "forged-sig",
			expectedStatus: http.StatusAccepted,
			expectedActor:  "",
		},
		{
			name:           "Bypass GET with ActivityPub Content-Type and No Signature",
			method:         http.MethodGet,
			path:           "https://sprezz.net/actor",
			contentType:    "application/ld+json",
			signature:      "",
			expectedStatus: http.StatusAccepted,
			expectedActor:  "",
		},
		{
			name:           "Verify GET with ActivityPub Content-Type and Valid Signature",
			method:         http.MethodGet,
			path:           "https://sprezz.net/actor",
			contentType:    "application/activity+json",
			signature:      "valid-sig",
			expectedStatus: http.StatusAccepted,
			expectedActor:  "https://remote-actor.com",
		},
		{
			name:           "Reject GET with ActivityPub Content-Type and Invalid Signature",
			method:         http.MethodGet,
			path:           "https://sprezz.net/actor",
			contentType:    "application/activity+json",
			signature:      "forged-sig",
			expectedStatus: http.StatusUnauthorized,
			expectedActor:  "",
		},
		{
			name:           "Bypass Signature Validator on Well-Known Paths",
			method:         http.MethodPost,
			path:           "https://sprezz.net/.well-known/webfinger",
			contentType:    "application/activity+json",
			signature:      "forged-sig",
			expectedStatus: http.StatusAccepted,
			expectedActor:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.signature != "" {
				req.Header.Set("Signature", tt.signature)
			}
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			// Handled response validation loops via a distinct single-responsibility helper block
			assertResponse(t, tt.name, rr, tt.expectedStatus, tt.expectedActor)
		})
	}
}

// assertResponse decouples validation loops from the test runner loop context to achieve absolute flat logic profiles.
func assertResponse(t *testing.T, name string, rr *httptest.ResponseRecorder, expectedStatus int, expectedActor string) {
	if rr.Code != expectedStatus {
		t.Errorf("Signature validation handling failure for %q: got status %d, want %d", name, rr.Code, expectedStatus)
	}

	if expectedStatus == http.StatusAccepted {
		extractedActor := rr.Header().Get("X-Authenticated-Actor")
		if extractedActor != expectedActor {
			t.Errorf("Context propagation mismatch in %q: got %q, want %q", name, extractedActor, expectedActor)
		}
	}
}
