package model

import (
	"fmt"
	"strings"
)

const (
	// ActorPathSegment represents the immutable segment utilized for actor resource paths.
	ActorPathSegment = "actor"
)

// ActorIRI builds a canonical Actor IRI based on the domain and UUID.
func ActorIRI(domain string, uuid string) string {
	return fmt.Sprintf("https://%s/%s/%s", domain, ActorPathSegment, uuid)
}

// ActorPrefixMatch builds the database/SQL prefix query match template for a given tenant domain.
func ActorPrefixMatch(domain string) string {
	return fmt.Sprintf("https://%s/%s/%%", domain, ActorPathSegment)
}

// HasActorPrefix checks if an IRI has the canonical actor path structure for a given domain.
func HasActorPrefix(iri, domain string) bool {
	prefix := fmt.Sprintf("https://%s/%s/", domain, ActorPathSegment)
	return strings.HasPrefix(iri, prefix)
}

// IsActorPath checks if a path's first segment is the actor path segment.
func IsActorPath(firstSegment string) bool {
	return firstSegment == ActorPathSegment || firstSegment == "actors" // Support legacy "actors" matches for compatibility during transition
}

// ActorProfile represents a local identity reconstructed from the RDF Quad Store graph history [source: 4].
type ActorProfile struct {
	UUID         string `json:"uuid"`           // Stable UUIDv4 identifier string [source: 4]
	IRI          string `json:"iri"`            // Canonical global ActivityPub Actor URI [source: 4]
	Username     string `json:"username"`       // Extracted local text-based username handle [source: 4]
	PublicKeyPEM string `json:"public_key_pem"` // Reconstructed signing key string [source: 4]
	NomadGUID    string `json:"nomad_guid"`     // Zot6 global identifier string; empty if vanilla AP [source: 4]
}

// ActorDualKeys maintains the long-term private key parameters for outbound federation [source: 4].
type ActorDualKeys struct {
	PrivateKeyRSAPEM     string
	PrivateKeyEd25519PEM string
}

// NomadicIdentity manages clone parameters for cross-server channel synchronization [source: 8].
type NomadicIdentity struct {
	GUID               string
	PrimaryHubURL      string
	MasterPublicKeyPEM string
	ClonedHubs         []string
}
