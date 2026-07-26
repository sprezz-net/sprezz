package model

// ActorProfile represents a local identity reconstructed from the RDF Quad Store graph history.
type ActorProfile struct {
	UUID         string `json:"uuid"`           // Stable UUIDv4 identifier string
	IRI          string `json:"iri"`            // Canonical global ActivityPub Actor URI (https://<domain>/actor/<uuidv4>)
	Username     string `json:"username"`       // Extracted local text-based username handle
	PublicKeyPEM string `json:"public_key_pem"` // Reconstructed signing key string
	NomadGUID    string `json:"nomad_guid"`     // Zot6 global identifier string; empty if vanilla AP
}
