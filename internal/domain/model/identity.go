package model

import (
	"fmt"
	"strings"
)

// ActorProfile represents a local identity reconstructed from the RDF Quad Store graph history.
type ActorProfile struct {
	UUID         string `json:"uuid"`           // Stable UUIDv4 identifier string
	IRI          string `json:"iri"`            // Canonical global ActivityPub Actor URI
	Username     string `json:"username"`       // Extracted local text-based username handle
	PublicKeyPEM string `json:"public_key_pem"` // Reconstructed signing key string
	NomadGUID    string `json:"nomad_guid"`     // Zot6 global identifier string; empty if vanilla AP
}

// ActorDualKeys maintains the long-term private key parameters for outbound federation.
type ActorDualKeys struct {
	PrivateKeyRSAPEM     string
	PrivateKeyEd25519PEM string
}

// NomadicIdentity manages clone parameters for cross-server channel synchronization.
type NomadicIdentity struct {
	GUID               string
	PrimaryHubURL      string
	MasterPublicKeyPEM string
	ClonedHubs         []string
}

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
