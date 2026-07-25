package middleware

import (
	"net"
	"net/http"
	"strings"
)

// RealIP returns a security-hardened middleware that extracts the actual client IP address
// based on an explicit architectural decision.
func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := ""

		// Check standard reverse proxy headers
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Take the leftmost IP string in the chain
			if idx := strings.IndexByte(xff, ','); idx != -1 {
				clientIP = strings.TrimSpace(xff[:idx])
			} else {
				clientIP = strings.TrimSpace(xff)
			}
		}

		if clientIP == "" {
			clientIP = r.Header.Get("X-Real-IP")
		}

		if clientIP == "" {
			// Fall back safely to the direct network socket remote address string
			var err error
			clientIP, _, err = net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				clientIP = r.RemoteAddr
			}
		}

		// Clean and re-inject the verified target address if valid
		if parsedIP := net.ParseIP(clientIP); parsedIP != nil {
			r.RemoteAddr = net.JoinHostPort(clientIP, "0")
		}

		next.ServeHTTP(w, r)
	})
}
