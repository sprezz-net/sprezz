package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sprezz/internal/domain/ports"
)

// Type contextKey is defined in tenant.go
const AuthenticatedActorKey contextKey = "authenticated_actor"

// SignatureVerifier matches your production cryptographic struct signature exactly.
type SignatureVerifier interface {
	Verify(r *http.Request, body []byte) error
}

type SignatureValidator struct {
	verifier SignatureVerifier
	storage  ports.StoragePort
}

func NewSignatureValidator(verifier SignatureVerifier, storage ports.StoragePort) *SignatureValidator {
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

		// 1. Read the body bytes to pass to the verifier
		var bodyBytes []byte
		if r.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(r.Body)
			if err != nil {
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

		// 3. Resolve the remote actor identity parameters from the request or request context
		// (Assuming your verifier injects or handles it, or extracting it from payload context)
		actorIRI := r.Header.Get("X-Actor-IRI") // Fallback check or customized metadata string extraction
		if actorIRI == "" {
			actorIRI = "authenticated-federated-actor" // Default marker fallback
		}

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
