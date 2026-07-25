package middleware

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const TenantDomainKey contextKey = "tenant_domain"

type TenantConfig struct {
	TenantDomains []string
}

// TenantValidator enforces that the incoming request host is explicitly registered.
type TenantValidator struct {
	allowedDomains map[string]struct{}
}

func NewTenantValidator(cfg TenantConfig) *TenantValidator {
	domains := make(map[string]struct{}, len(cfg.TenantDomains))
	for _, domain := range cfg.TenantDomains {
		domains[strings.ToLower(strings.TrimSpace(domain))] = struct{}{}
	}
	return &TenantValidator{allowedDomains: domains}
}

// Handler is a native Chi-compatible middleware that validates multi-tenant boundaries.
func (v *TenantValidator) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if idx := strings.IndexByte(host, ':'); idx != -1 {
			host = host[:idx]
		}
		host = strings.ToLower(strings.TrimSpace(host))

		if _, allowed := v.allowedDomains[host]; !allowed {
			http.Error(w, "Access Denied: Host domain is not an authorized tenant on this instance", http.StatusNotFound)
			return
		}

		// Inject the validated domain string into the request context for lower layers to read
		ctx := context.WithValue(r.Context(), TenantDomainKey, host)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetTenantDomain extracts the pre-validated tenant string safely from the context.
func GetTenantDomain(ctx context.Context) string {
	if val, ok := ctx.Value(TenantDomainKey).(string); ok {
		return val
	}
	return ""
}
