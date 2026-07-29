package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

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
		// 1. Exclude any well-known endpoints
		if strings.HasPrefix(r.URL.Path, "/.well-known/") {
			next.ServeHTTP(w, r)
			return
		}

		// 2. Determine if it is an ActivityPub Content-Type
		contentType := r.Header.Get("Content-Type")
		isActivityPub := strings.Contains(contentType, "application/activity+json") ||
			strings.Contains(contentType, "application/ld+json")

		// 3. Strict Content-Type check for S2S/C2S ActivityPub collections on POST
		if r.Method == http.MethodPost {
			if contentType == "" {
				http.Error(w, "Bad Request: Missing Content-Type header", http.StatusBadRequest)
				return
			}

			path := r.URL.Path
			isCollectionPost := strings.HasSuffix(path, "/inbox") ||
				strings.HasSuffix(path, "/outbox") ||
				strings.HasSuffix(path, "/followers") ||
				strings.HasSuffix(path, "/following") ||
				strings.HasSuffix(path, "/likes") ||
				strings.HasSuffix(path, "/shares") ||
				strings.HasSuffix(path, "/replies") ||
				path == "/inbox" ||
				path == "/outbox"

			if isCollectionPost && !isActivityPub {
				http.Error(w, "Unsupported Media Type: POST requests to ActivityPub collections must use standard ActivityPub MIME types", http.StatusUnsupportedMediaType)
				return
			}
		}

		// 4. Method-based signature validation routing
		switch r.Method {
		case http.MethodPost:
			// For POST: verify signatures on any URL, but only when content-type is activitypub
			if !isActivityPub {
				next.ServeHTTP(w, r)
				return
			}
			// If it is ActivityPub, signature is mandatory. Let's make sure a signature header exists
			sigHeader := r.Header.Get("Signature")
			if sigHeader == "" {
				http.Error(w, "Unauthorized: Invalid or missing HTTP Signature header", http.StatusUnauthorized)
				return
			}
		case http.MethodGet:
			// For GET: only verify signatures when content-type is activitypub AND signature header is present
			if !isActivityPub {
				next.ServeHTTP(w, r)
				return
			}
			sigHeader := r.Header.Get("Signature")
			if sigHeader == "" {
				// Signature is optional for GET, so bypass verification if absent
				next.ServeHTTP(w, r)
				return
			}
		default:
			// For other methods, bypass verification
			next.ServeHTTP(w, r)
			return
		}

		// Securely clear any pre-existing untrusted client-supplied header tracking parameters
		// to eliminate identity spoofing vectors via malicious header injection.
		r.Header.Del("X-Actor-IRI")

		// 4. Safely read and clone the body bytes to pass to the verifier
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

		// 5. Perform cryptographic verification pass against your production verifier
		if err := v.verifier.Verify(r, bodyBytes); err != nil {
			http.Error(w, "Unauthorized: Invalid or missing HTTP Signature header", http.StatusUnauthorized)
			return
		}

		// 6. Extract the cryptographically verified actor identifier set exclusively by the verifier
		actorIRI := r.Header.Get("X-Actor-IRI")
		if actorIRI == "" {
			http.Error(w, "Unauthorized: Signature verifier failed to assert actor identity metadata", http.StatusUnauthorized)
			return
		}

		// 7. Enforce domain policy isolation blocks against the cryptographically extracted actor
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
