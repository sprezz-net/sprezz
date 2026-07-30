package model

import "strings"

const (
	// Canonical namespace URIs
	NamespaceActivityStreams = "https://www.w3.org/ns/activitystreams#"
	NamespaceRDF             = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	NamespaceSecurity        = "https://w3id.org/security#"
	NamespaceZot             = "http://purl.org/zot/protocol/"

	// Base document domain prefix URLs for contexts mapping
	BaseW3OrgHTTPS = "https://w3.org"
	BaseW3OrgHTTP  = "http://w3.org"
	BaseW3IDHTTPS  = "https://w3id.org"
	BaseW3IDHTTP   = "http://w3id.org"

	// Short type names used in unmarshaled JSON payloads
	ShortPerson       = "Person"
	ShortService      = "Service"
	ShortGroup        = "Group"
	ShortOrganization = "Organization"
	ShortApplication  = "Application"

	// Activity Verb short names
	ShortFollow   = "Follow"
	ShortAccept   = "Accept"
	ShortReject   = "Reject"
	ShortCreate   = "Create"
	ShortLike     = "Like"
	ShortDislike  = "Dislike"
	ShortAnnounce = "Announce"
	ShortUndo     = "Undo"
	ShortDelete   = "Delete"
	ShortUpdate   = "Update"
	ShortAdd      = "Add"
	ShortRemove   = "Remove"
	ShortJoin     = "Join"
	ShortLeave    = "Leave"
	ShortQuestion = "Question"

	// Object short names
	ShortNote      = "Note"
	ShortTombstone = "Tombstone"

	// Canonical vocabulary URIs used to scan actor graphs
	PredicatePreferredUsername = NamespaceActivityStreams + "preferredUsername"
	PredicatePublicKeyPem      = NamespaceSecurity + "publicKeyPem"
	PredicateNomadGUID         = NamespaceZot + "6.0#guid"
	PredicateZotGUID           = NamespaceZot + "guid"

	RDFType = NamespaceRDF + "type"

	// ActivityPub Actor Types (dynamically constructed)
	ActorPerson       = NamespaceActivityStreams + ShortPerson
	ActorService      = NamespaceActivityStreams + ShortService
	ActorGroup        = NamespaceActivityStreams + ShortGroup
	ActorOrganization = NamespaceActivityStreams + ShortOrganization
	ActorApplication  = NamespaceActivityStreams + ShortApplication

	// ActivityPub Object/Activity Types (dynamically constructed)
	TypeFollow    = NamespaceActivityStreams + ShortFollow
	TypeAccept    = NamespaceActivityStreams + ShortAccept
	TypeReject    = NamespaceActivityStreams + ShortReject
	TypeCreate    = NamespaceActivityStreams + ShortCreate
	TypeLike      = NamespaceActivityStreams + ShortLike
	TypeDislike   = NamespaceActivityStreams + ShortDislike
	TypeAnnounce  = NamespaceActivityStreams + ShortAnnounce
	TypeUndo      = NamespaceActivityStreams + ShortUndo
	TypeDelete    = NamespaceActivityStreams + ShortDelete
	TypeUpdate    = NamespaceActivityStreams + ShortUpdate
	TypeAdd       = NamespaceActivityStreams + ShortAdd
	TypeRemove    = NamespaceActivityStreams + ShortRemove
	TypeJoin      = NamespaceActivityStreams + ShortJoin
	TypeLeave     = NamespaceActivityStreams + ShortLeave
	TypeQuestion  = NamespaceActivityStreams + ShortQuestion
	TypeNote      = NamespaceActivityStreams + ShortNote
	TypeTombstone = NamespaceActivityStreams + ShortTombstone

	// Vocabulary predicates
	PredicateInbox       = NamespaceActivityStreams + "inbox"
	PredicateSharedInbox = NamespaceActivityStreams + "sharedInbox"
	PredicateFollower    = NamespaceActivityStreams + "follower"
	PredicateAccepted    = NamespaceActivityStreams + "accepted"
	PredicateRejected    = NamespaceActivityStreams + "rejected"

	// ActivityPub Public Addressing Target
	PublicAudience = NamespaceActivityStreams + "Public"

	// SuffixMainKey represents the default key ID suffix for AP actor keys
	SuffixMainKey = "#main-key"

	// ActivityPub Collection Types
	CollectionRegular     = NamespaceActivityStreams + "Collection"
	CollectionOrdered     = NamespaceActivityStreams + "OrderedCollection"
	CollectionPageRegular = NamespaceActivityStreams + "CollectionPage"
	CollectionPageOrdered = NamespaceActivityStreams + "OrderedCollectionPage"
)

// IsActorType checks if a given URI string is one of the valid ActivityPub actor types.
func IsActorType(uri string) bool {
	return uri == ActorPerson ||
		uri == ActorService ||
		uri == ActorGroup ||
		uri == ActorOrganization ||
		uri == ActorApplication
}

// IsCollectionType checks if a given URI string matches standard ActivityPub collection types.
func IsCollectionType(uri string) bool {
	cleanURI := strings.Trim(uri, `"'`)
	return cleanURI == CollectionRegular ||
		cleanURI == CollectionOrdered ||
		cleanURI == CollectionPageRegular ||
		cleanURI == CollectionPageOrdered
}

// IsGroupOrCollection checks if a given type string is a Group or any Collection variety (case-insensitive).
func IsGroupOrCollection(t string) bool {
	clean := strings.ToLower(strings.Trim(t, `"'`))
	return strings.Contains(clean, "group") || strings.Contains(clean, "collection")
}
