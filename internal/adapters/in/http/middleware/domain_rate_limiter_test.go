package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sprezz/internal/adapters/in/http/middleware"
)

func TestDomainRateLimitMiddleware_Handler(t *testing.T) {
	// Configure middleware to allow only 2 requests per 100ms per domain
	mw := middleware.NewDomainRateLimitMiddleware(2, 100*time.Millisecond)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	handler := mw.Handler(nextHandler)

	// Helper to run requests
	runRequest := func(method, path, actorIRI string) int {
		req := httptest.NewRequest(method, path, nil)
		if actorIRI != "" {
			ctx := context.WithValue(req.Context(), middleware.AuthenticatedActorKey, actorIRI)
			req = req.WithContext(ctx)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	// 1. Request with no authenticated actor should bypass rate limiter
	if status := runRequest(http.MethodPost, "/inbox", ""); status != http.StatusAccepted {
		t.Errorf("Expected bypass for anonymous request, got status %d", status)
	}

	// 2. Request 1 for domain "example.com" -> allowed
	if status := runRequest(http.MethodPost, "/inbox", "https://example.com/actor/alice"); status != http.StatusAccepted {
		t.Errorf("Expected first request for domain 'example.com' to be accepted, got %d", status)
	}

	// 3. Request 2 for subdomain "sub.example.com" -> allowed (maps to example.com registrable domain)
	if status := runRequest(http.MethodPost, "/inbox", "https://sub.example.com/actor/bob"); status != http.StatusAccepted {
		t.Errorf("Expected second request for domain 'example.com' (subdomain) to be accepted, got %d", status)
	}

	// 4. Request 3 for domain "example.com" on a non-collection POST route -> should bypass rate limiter (e.g. media upload)
	if status := runRequest(http.MethodPost, "/media/upload", "https://sub2.example.com/actor/charlie"); status != http.StatusAccepted {
		t.Errorf("Expected media upload to bypass rate limiter, got %d", status)
	}

	// 5. Request 4 for domain "example.com" on a GET inbox route -> should bypass rate limiter
	if status := runRequest(http.MethodGet, "/inbox", "https://sub3.example.com/actor/charlie"); status != http.StatusAccepted {
		t.Errorf("Expected GET inbox to bypass rate limiter, got %d", status)
	}

	// 6. Request 5 for domain "example.com" on a real POST inbox route -> rate limited (exceeded limit of 2, subdomains grouped)
	if status := runRequest(http.MethodPost, "/inbox", "https://another-sub.example.com/actor/charlie"); status != http.StatusTooManyRequests {
		t.Errorf("Expected third POST request to inbox for domain 'example.com' to be rate limited, got %d", status)
	}

	// 7. Request 1 for domain "other.com" -> allowed (different domain is independent)
	if status := runRequest(http.MethodPost, "/inbox", "https://other.com/actor/dan"); status != http.StatusAccepted {
		t.Errorf("Expected first request for domain 'other.com' to be accepted, got %d", status)
	}

	// 8. Wait for window to expire (100ms + buffer)
	time.Sleep(150 * time.Millisecond)

	// 9. Request 6 for domain "example.com" -> allowed again
	if status := runRequest(http.MethodPost, "/inbox", "https://example.com/actor/alice"); status != http.StatusAccepted {
		t.Errorf("Expected request after window expiration to be accepted, got %d", status)
	}
}
