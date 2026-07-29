package httpclient

import (
	"net/http"
	"time"
)

const UserAgent = "Sprezz-Hex-QuadStore/2.0"

// userAgentTransport wraps an existing http.RoundTripper and injects the predefined User-Agent.
type userAgentTransport struct {
	base http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", UserAgent)
	return t.base.RoundTrip(req)
}

// New returns an *http.Client configured with a 10-second timeout and the User-Agent transport.
func New() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &userAgentTransport{
			base: http.DefaultTransport,
		},
	}
}
