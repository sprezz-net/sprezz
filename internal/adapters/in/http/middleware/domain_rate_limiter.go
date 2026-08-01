package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"sprezz/internal/domain/model"
	"sprezz/internal/pkg/httputil"
)

// DomainRateLimiter implements a thread-safe sliding window rate limiter based on domain strings.
type DomainRateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewDomainRateLimiter instantiates a new domain sliding window rate limiter.
func NewDomainRateLimiter(limit int, window time.Duration) *DomainRateLimiter {
	return &DomainRateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow checks if the request under a given domain is within the allowed limit.
func (l *DomainRateLimiter) Allow(domain string) bool {
	if domain == "" {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	times := l.requests[domain]
	valid := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= l.limit {
		l.requests[domain] = valid
		return false
	}

	valid = append(valid, now)
	l.requests[domain] = valid
	return true
}

// DomainRateLimitMiddleware limits requests from federated servers based on their domain.
// Security Note: It acts explicitly on the registrable domain (top-level + 1 components)
// of the cryptographically-verified sending actor's IRI (e.g. extracted from 'X-Actor-IRI'
// after signature validation), protecting against randomized subdomain floods while
// completely isolating rate limits across independent domains.
type DomainRateLimitMiddleware struct {
	limiter *DomainRateLimiter
}

// NewDomainRateLimitMiddleware instantiates a new configurable domain rate limiting middleware.
func NewDomainRateLimitMiddleware(limit int, window time.Duration) *DomainRateLimitMiddleware {
	return &DomainRateLimitMiddleware{
		limiter: NewDomainRateLimiter(limit, window),
	}
}

// Handler intercepts HTTP requests and enforces domain-based rate limiting on authenticated actors targeting collection routes.
func (m *DomainRateLimitMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only enforce rate-limiting on POST requests targeting collection endpoints
		if r.Method == http.MethodPost && isCollectionPath(r.URL.Path) {
			actorIRI := GetAuthenticatedActor(r.Context())
			if actorIRI != "" {
				domain := extractActorDomain(actorIRI)
				if domain != "" && !m.limiter.Allow(domain) {
					http.Error(w, "Too Many Requests: Rate limit exceeded for sender's domain", http.StatusTooManyRequests)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isCollectionPath checks if the given path targets an ActivityPub collection using model.IsCollectionPathSuffix.
func isCollectionPath(path string) bool {
	path = strings.TrimPrefix(path, "/")
	path = strings.ToLower(path)
	if model.IsCollectionPathSuffix(path) {
		return true
	}
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return false
	}
	lastSegment := path[idx+1:]
	return model.IsCollectionPathSuffix(lastSegment)
}

// extractActorDomain safely parses out the host/domain from an actor IRI using httputil prefix constants,
// extracting only the top-level + 1 domain components (registrable domain) to defeat randomized subdomain floods.
func extractActorDomain(actorIRI string) string {
	if !strings.HasPrefix(actorIRI, httputil.HTTPSPrefix) && !strings.HasPrefix(actorIRI, httputil.HTTPPrefix) {
		return ""
	}
	parts := strings.Split(actorIRI, "/")
	if len(parts) < 3 {
		return ""
	}
	host := parts[2]
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)

	// Extract last and second-to-last components (e.g. "sub.example.com" -> "example.com")
	hostParts := strings.Split(host, ".")
	if len(hostParts) >= 2 {
		return hostParts[len(hostParts)-2] + "." + hostParts[len(hostParts)-1]
	}
	return host
}
