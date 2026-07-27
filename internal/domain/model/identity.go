package model

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
