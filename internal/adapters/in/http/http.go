package http

import (
	"net/http"
	"strings"
)

// RequestHost extracts the clean hostname from the Request, removing any port numbers.
func RequestHost(r *http.Request) string {
	host := r.Host
	if parts := strings.Split(host, ":"); len(parts) > 0 {
		return parts[0]
	}
	return host
}
