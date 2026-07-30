package httpclient

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsPrivateOrLocalIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		// Loopback
		{"127.0.0.1", true},
		{"::1", true},

		// Private (RFC 1918)
		{"10.0.0.1", true},
		{"172.16.31.254", true},
		{"192.168.1.50", true},

		// Carrier-Grade NAT (RFC 6598)
		{"100.64.0.1", true},

		// Link-Local (APIPA)
		{"169.254.169.254", true},
		{"fe80::1", true},

		// Unspecified
		{"0.0.0.0", true},
		{"::", true},

		// Multicast
		{"224.0.0.1", true},
		{"ff02::1", true},

		// Public IP addresses
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"104.244.42.1", false},
		{"2606:4700:4700::1111", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("failed to parse test IP: %s", tt.ip)
		}
		result := isPrivateOrLocalIP(ip)
		if result != tt.expected {
			t.Errorf("isPrivateOrLocalIP(%s) = %v; want %v", tt.ip, result, tt.expected)
		}
	}
}

func TestSecureClient_BlocksPrivateAddresses(t *testing.T) {
	// Start a local test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Success"))
	}))
	defer ts.Close()

	// Create our secure client
	client := New()

	// Try to fetch from the local test server (which runs on 127.0.0.1 / loopback)
	_, err := client.Get(ts.URL)
	if err == nil {
		t.Fatal("expected request to local test server to fail, but it succeeded")
	}

	// Verify that the failure was indeed due to the SSRF block
	expectedErrSub := "private or local address is forbidden"
	errStr := err.Error()
	if !contains(errStr, expectedErrSub) {
		t.Errorf("expected error to contain %q, but got: %s", expectedErrSub, errStr)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
