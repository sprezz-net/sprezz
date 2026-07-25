package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sprezz/internal/adapters/in/http/middleware"
)

func TestTenantValidator_Handler(t *testing.T) {
	cfg := middleware.TenantConfig{
		TenantDomains: []string{"sprezz.net", "social.example.org"},
	}
	validator := middleware.NewTenantValidator(cfg)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := middleware.GetTenantDomain(r.Context())
		w.Header().Set("X-Tenant-Domain", domain)
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := validator.Handler(nextHandler)

	tests := []struct {
		name           string
		requestHost    string
		expectedStatus int
		expectedDomain string
	}{
		{
			name:           "Valid Exact Tenant Match",
			requestHost:    "sprezz.net",
			expectedStatus: http.StatusOK,
			expectedDomain: "sprezz.net",
		},
		{
			name:           "Valid Case-Insensitive Match",
			requestHost:    "SOCIAL.example.org",
			expectedStatus: http.StatusOK,
			expectedDomain: "social.example.org",
		},
		{
			name:           "Valid Port Stripping Optimization Pass",
			requestHost:    "sprezz.net:8080",
			expectedStatus: http.StatusOK,
			expectedDomain: "sprezz.net",
		},
		{
			name:           "Rejected Unauthorized Domain Request",
			requestHost:    "malicious-site.com",
			expectedStatus: http.StatusNotFound,
			expectedDomain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "https://sprezz.net", nil)
			req.Host = tt.requestHost
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Host validation routing failure for %q: got status %d, want %d", tt.requestHost, rr.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				extractedDomain := rr.Header().Get("X-Tenant-Domain")
				if extractedDomain != tt.expectedDomain {
					t.Errorf("Context propagation mismatch: got %q, want %q", extractedDomain, tt.expectedDomain)
				}
			}
		})
	}
}
