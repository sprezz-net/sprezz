package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/pkg/httputil"
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

func (v *SignatureValidator) isWellKnown(path string) bool {
	return strings.HasPrefix(path, "/.well-known/")
}

func (v *SignatureValidator) isActivityPubMime(contentType string) bool {
	return strings.Contains(contentType, httputil.ContentTypeActivityJSON) ||
		strings.Contains(contentType, "application/ld+json")
}

func (v *SignatureValidator) isCollectionPost(path string) bool {
	path = strings.TrimPrefix(path, "/")
	path = strings.ToLower(path)
	if model.IsCollectionPathSuffix(path) {
		return true
	}
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return false
	}
	return model.IsCollectionPathSuffix(path[idx+1:])
}

func (v *SignatureValidator) checkPostMime(r *http.Request, contentType string, isActivityPub bool) (int, string) {
	if r.Method != http.MethodPost {
		return 0, ""
	}
	if contentType == "" {
		return http.StatusBadRequest, "Bad Request: Missing Content-Type header"
	}
	if v.isCollectionPost(r.URL.Path) && !isActivityPub {
		return http.StatusUnsupportedMediaType, "Unsupported Media Type: POST requests to ActivityPub collections must use standard ActivityPub MIME types"
	}
	return 0, ""
}

func (v *SignatureValidator) shouldVerifySignature(r *http.Request, isActivityPub bool) (bool, int, string) {
	sigHeader := r.Header.Get(httputil.HeaderSignature)
	switch r.Method {
	case http.MethodPost:
		if !isActivityPub {
			return false, 0, ""
		}
		if sigHeader == "" {
			return false, http.StatusUnauthorized, "Unauthorized: Invalid or missing HTTP Signature header"
		}
		return true, 0, ""
	case http.MethodGet:
		if !isActivityPub || sigHeader == "" {
			return false, 0, ""
		}
		return true, 0, ""
	default:
		return false, 0, ""
	}
}

func (v *SignatureValidator) readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		r.Body = io.NopCloser(bytes.NewReader(nil))
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, nil
}

func (v *SignatureValidator) verifyAndAuthorize(ctx context.Context, r *http.Request, bodyBytes []byte) (string, int, string) {
	if err := v.verifier.Verify(r, bodyBytes); err != nil {
		return "", http.StatusUnauthorized, "Unauthorized: Invalid or missing HTTP Signature header"
	}

	actorIRI := r.Header.Get("X-Actor-IRI")
	if actorIRI == "" {
		return "", http.StatusUnauthorized, "Unauthorized: Signature verifier failed to assert actor identity metadata"
	}

	blocked, err := v.storage.IsDomainBlocked(ctx, actorIRI)
	if err != nil || blocked {
		return "", http.StatusForbidden, "Forbidden: Blocked federation domain identity match"
	}

	return actorIRI, 0, ""
}

func (v *SignatureValidator) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v.isWellKnown(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		contentType := r.Header.Get(httputil.HeaderContentType)
		isActivityPub := v.isActivityPubMime(contentType)

		if status, msg := v.checkPostMime(r, contentType, isActivityPub); status != 0 {
			http.Error(w, msg, status)
			return
		}

		shouldVerify, status, msg := v.shouldVerifySignature(r, isActivityPub)
		if status != 0 {
			http.Error(w, msg, status)
			return
		}
		if !shouldVerify {
			next.ServeHTTP(w, r)
			return
		}

		r.Header.Del("X-Actor-IRI")

		bodyBytes, err := v.readBody(r)
		if err != nil {
			http.Error(w, "Bad Request: Unable to read request payload", http.StatusBadRequest)
			return
		}

		actorIRI, status, msg := v.verifyAndAuthorize(r.Context(), r, bodyBytes)
		if status != 0 {
			http.Error(w, msg, status)
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
