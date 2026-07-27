package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sprezz/internal/domain/port"
)

// The shared package-visible contextKey is natively inherited from tenant.go.
const AuthenticatedActorKey contextKey = "authenticated_actor"

// SignatureVerifier matches your production cryptographic struct signature exactly.
type SignatureVerifier interface {
	Verify(r *http.Request, body []byte) error
}

type SignatureValidator struct {
	verifier SignatureVerifier
	storage  port.StoragePort
}

func NewSignatureValidator(verifier SignatureVerifier, storage port.StoragePort) *SignatureValidator {
	return &SignatureValidator{
		verifier: verifier,
		storage:  storage,
	}
}

func (v *SignatureValidator) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		// Securely clear any pre-existing untrusted client-supplied header tracking parameters
		// to eliminate identity spoofing vectors via malicious header injection.
		r.Header.Del("X-Actor-IRI")

		// 1. Safely read and clone the body bytes to pass to the verifier
		var bodyBytes []byte
		if r.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(r.Body)
			// FIXED: Re-instantiate a valid empty reader closer pool on failure to eliminate memory leaks
			if err != nil {
				r.Body = io.NopCloser(bytes.NewReader(nil))
				http.Error(w, "Bad Request: Unable to read request payload", http.StatusBadRequest)
				return
			}
			// Restore the stream immediately so lower-level handlers can read it later
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		// 2. Perform cryptographic verification pass against your production verifier
		if err := v.verifier.Verify(r, bodyBytes); err != nil {
			http.Error(w, "Unauthorized: Invalid or missing HTTP Signature header", http.StatusUnauthorized)
			return
		}

		// 3. Extract the cryptographically verified actor identifier set exclusively by the verifier
		actorIRI := r.Header.Get("X-Actor-IRI")
		if actorIRI == "" {
			http.Error(w, "Unauthorized: Signature verifier failed to assert actor identity metadata", http.StatusUnauthorized)
			return
		}

		// 4. Enforce domain policy isolation blocks against the cryptographically extracted actor
		blocked, err := v.storage.IsDomainBlocked(r.Context(), actorIRI)
		if err != nil || blocked {
			http.Error(w, "Forbidden: Blocked federation domain identity match", http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), AuthenticatedActorKey, actorIRI)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetAuthenticatedActor(ctx context.Context) string {
	if val, ok := ctx.Value(AuthenticatedActorKey).(string); ok {
		return val
	}
	return ""
}
