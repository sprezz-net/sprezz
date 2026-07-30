package middleware

import (
	"context"
	"net/http"
	"strings"

	"sprezz/internal/domain/model"
)

type contextKey string

const TenantDomainKey contextKey = "tenant_domain"

type TenantConfig struct {
	TenantDomainToID map[string]int32
}

// TenantValidator enforces that the incoming request host is explicitly registered.
type TenantValidator struct {
	tenantIDs map[string]int32
}

func NewTenantValidator(cfg TenantConfig) *TenantValidator {
	domains := make(map[string]int32, len(cfg.TenantDomainToID))
	for domain, id := range cfg.TenantDomainToID {
		domains[strings.ToLower(strings.TrimSpace(domain))] = id
	}
	return &TenantValidator{tenantIDs: domains}
}

// Handler is a native Chi-compatible middleware that validates multi-tenant boundaries.
func (v *TenantValidator) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if idx := strings.IndexByte(host, ':'); idx != -1 {
			host = host[:idx]
		}
		host = strings.ToLower(strings.TrimSpace(host))

		tenantID, allowed := v.tenantIDs[host]
		if !allowed {
			http.Error(w, "Access Denied: Host domain is not an authorized tenant on this instance", http.StatusNotFound)
			return
		}

		// Inject the validated domain string and tenant ID into the request context for lower layers to read
		ctx := context.WithValue(r.Context(), TenantDomainKey, host)
		ctx = context.WithValue(ctx, model.TenantIDKey, tenantID)
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
