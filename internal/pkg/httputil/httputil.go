package httputil

import (
	"net/http"
	"strings"
)

const (
	// URI Protocols
	HTTPSPrefix = "https://"
	HTTPPrefix  = "http://"

	// Standard HTTP Headers
	HeaderContentType = "Content-Type"
	HeaderAccept      = "Accept"
	HeaderSignature   = "Signature"
	HeaderDigest      = "Digest"
	HeaderDate        = "Date"
	HeaderHost        = "Host"
	HeaderCollectionSynchronization = "Collection-Synchronization"

	// MIME Types
	ContentTypeActivityJSON = "application/activity+json"
	ContentTypeLDJSON       = "application/ld+json"
	ContentTypeJRDJSON      = "application/jrd+json"
	ContentTypeJSON         = "application/json"

	// Accept Headers
	AcceptActivityPub = "application/activity+json, application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\", application/jrd+json"
)

// CleanHost extracts the clean hostname from the Request, removing any port numbers.
func CleanHost(host string) string {
	if parts := strings.Split(host, ":"); len(parts) > 0 {
		return parts[0]
	}
	return host
}

// RequestHost extracts the clean hostname from an http.Request.
func RequestHost(r *http.Request) string {
	return CleanHost(r.Host)
}
