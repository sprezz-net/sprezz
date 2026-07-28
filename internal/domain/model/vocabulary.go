package model

const (
	// Canonical vocabulary URIs used to scan actor graphs
	PredicatePreferredUsername = "https://www.w3.org/ns/activitystreams#preferredUsername"
	PredicatePublicKeyPem      = "https://w3id.org/security#publicKeyPem"
	PredicateNomadGUID         = "http://purl.org/zot/protocol/6.0#guid"

	RDFType = "http://www.w3.org/1999/02/22-rdf-syntax-ns#type"

	// ActivityPub Actor Types
	ActorPerson       = "https://www.w3.org/ns/activitystreams#Person"
	ActorService      = "https://www.w3.org/ns/activitystreams#Service"
	ActorGroup        = "https://www.w3.org/ns/activitystreams#Group"
	ActorOrganization = "https://www.w3.org/ns/activitystreams#Organization"
	ActorApplication  = "https://www.w3.org/ns/activitystreams#Application"
)

// IsActorType checks if a given URI string is one of the valid ActivityPub actor types.
func IsActorType(uri string) bool {
	return uri == ActorPerson ||
		uri == ActorService ||
		uri == ActorGroup ||
		uri == ActorOrganization ||
		uri == ActorApplication
}
