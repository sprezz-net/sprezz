package model

const (
	// JSON-LD Specific Key Names
	JSONLDContext = "@context"

	// Canonical context URIs
	ContextActivityStreams = "https://www.w3.org/ns/activitystreams"

	// Canonical namespace URIs
	NamespaceActivityStreams = "https://www.w3.org/ns/activitystreams#"
	NamespaceRDF             = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	NamespaceSecurity        = "https://w3id.org/security#"
	NamespaceZot             = "http://purl.org/zot/protocol/"
	NamespaceGoToSocial      = "https://gotosocial.org/ns#"

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
	ShortFollow     = "Follow"
	ShortAccept     = "Accept"
	ShortReject     = "Reject"
	ShortCreate     = "Create"
	ShortLike       = "Like"
	ShortDislike    = "Dislike"
	ShortAnnounce   = "Announce"
	ShortUndo       = "Undo"
	ShortDelete     = "Delete"
	ShortUpdate     = "Update"
	ShortAdd        = "Add"
	ShortRemove     = "Remove"
	ShortJoin       = "Join"
	ShortLeave      = "Leave"
	ShortQuestion   = "Question"
	ShortEmojiReact = "EmojiReact"

	// Object short names
	ShortNote               = "Note"
	ShortTombstone          = "Tombstone"
	ShortEmoji              = "Emoji"
	ShortQuoteRequest       = "QuoteRequest"
	ShortQuoteAuthorization = "QuoteAuthorization"

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
	TypeFollow             = NamespaceActivityStreams + ShortFollow
	TypeAccept             = NamespaceActivityStreams + ShortAccept
	TypeReject             = NamespaceActivityStreams + ShortReject
	TypeCreate             = NamespaceActivityStreams + ShortCreate
	TypeLike               = NamespaceActivityStreams + ShortLike
	TypeDislike            = NamespaceActivityStreams + ShortDislike
	TypeAnnounce           = NamespaceActivityStreams + ShortAnnounce
	TypeUndo               = NamespaceActivityStreams + ShortUndo
	TypeDelete             = NamespaceActivityStreams + ShortDelete
	TypeUpdate             = NamespaceActivityStreams + ShortUpdate
	TypeAdd                = NamespaceActivityStreams + ShortAdd
	TypeRemove             = NamespaceActivityStreams + ShortRemove
	TypeJoin               = NamespaceActivityStreams + ShortJoin
	TypeLeave              = NamespaceActivityStreams + ShortLeave
	TypeQuestion           = NamespaceActivityStreams + ShortQuestion
	TypeEmojiReact         = "http://litepub.social/ns#EmojiReact"
	TypeEmoji              = "http://joinmastodon.org/ns#Emoji"
	TypeNote               = NamespaceActivityStreams + ShortNote
	TypeTombstone          = NamespaceActivityStreams + ShortTombstone
	TypeQuoteRequest       = "https://w3id.org/fep/044f#QuoteRequest"
	TypeQuoteAuthorization = "https://w3id.org/fep/044f#QuoteAuthorization"

	// Vocabulary predicates
	PredicateInbox              = NamespaceActivityStreams + "inbox"
	PredicateSharedInbox        = NamespaceActivityStreams + "sharedInbox"
	PredicatePublicAppend       = "https://w3id.org/fep/400e/publicAppend"
	PredicateFollower           = NamespaceActivityStreams + "follower"
	PredicateAccepted           = NamespaceActivityStreams + "accepted"
	PredicateRejected           = NamespaceActivityStreams + "rejected"
	PredicateActor              = NamespaceActivityStreams + "actor"
	PredicateAttributedTo       = NamespaceActivityStreams + "attributedTo"
	PredicateObject             = NamespaceActivityStreams + "object"
	PredicateResult             = NamespaceActivityStreams + "result"
	PredicateTo                 = NamespaceActivityStreams + "to"
	PredicateCc                 = NamespaceActivityStreams + "cc"
	PredicateBto                = NamespaceActivityStreams + "bto"
	PredicateBcc                = NamespaceActivityStreams + "bcc"
	PredicateAudience           = NamespaceActivityStreams + "audience"
	PredicateFollowers          = NamespaceActivityStreams + "followers"
	PredicateEndTime            = NamespaceActivityStreams + "endTime"
	PredicateVoted              = NamespaceActivityStreams + "voted"
	PredicateLiked              = NamespaceActivityStreams + "liked"
	PredicateContext            = NamespaceActivityStreams + "context"
	PredicateContextHistory     = "https://w3id.org/fep/171b/contextHistory"
	PredicateQuote              = "https://w3id.org/fep/044f#quote"
	PredicateQuoteAuthorization = "https://w3id.org/fep/044f#quoteAuthorization"
	PredicateEmojiReactions     = "http://fedibird.com/ns#emojiReactions"
	PredicateInstrument         = NamespaceActivityStreams + "instrument"

	// GoToSocial specific predicates
	PredicateInteractingObject = NamespaceGoToSocial + "interactingObject"
	PredicateInteractionTarget = NamespaceGoToSocial + "interactionTarget"
	PredicateInteractionPolicy = NamespaceGoToSocial + "interactionPolicy"
	PredicateCanQuote          = NamespaceGoToSocial + "canQuote"
	PredicateManualApproval    = NamespaceGoToSocial + "manualApproval"
	PredicateAutomaticApproval = NamespaceGoToSocial + "automaticApproval"

	// Interaction policy values
	PolicyManual = "manual"

	// FEP-4ccd Pending Followers / Following Collection Predicates
	NamespacePending          = "https://purl.archive.org/socialweb/pending#"
	PredicatePendingFollowers = NamespacePending + "pendingFollowers"
	PredicatePendingFollowing = NamespacePending + "pendingFollowing"

	// Collection short names / slugs
	ShortInbox            = "inbox"
	ShortOutbox           = "outbox"
	ShortFollowers        = "followers"
	ShortFollowing        = "following"
	ShortPendingFollowers = "pendingFollowers"
	ShortPendingFollowing = "pendingFollowing"
	ShortBlocked          = "blocked"
	ShortBlocks           = "blocks"
	ShortLikes            = "likes"
	ShortShares           = "shares"
	ShortReplies          = "replies"
	ShortContext          = "context"
	ShortContextHistory   = "contextHistory"
	ShortFollowersSync    = "followers_synchronization"

	// URI Path Suffixes for routing and collection requests
	PathSuffixInbox            = "inbox"
	PathSuffixSharedInbox      = "inbox" // Decoupled shared inbox URI path slug
	PathSuffixOutbox           = "outbox"
	PathSuffixFollowers        = "followers"
	PathSuffixFollowing        = "following"
	PathSuffixPendingFollowers = "pendingFollowers"
	PathSuffixPendingFollowing = "pendingFollowing"
	PathSuffixBlocked          = "blocked"
	PathSuffixBlocks           = "blocks"
	PathSuffixLikes            = "likes"
	PathSuffixShares           = "shares"
	PathSuffixReplies          = "replies"
	PathSuffixContext          = "context"
	PathSuffixContextHistory   = "contextHistory"
	PathSuffixFollowersSync    = "followers_synchronization"

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

// IsShortActorType checks if a given short type string matches any of the valid ActivityPub actor types.
func IsShortActorType(t string) bool {
	return t == ShortPerson ||
		t == ShortService ||
		t == ShortGroup ||
		t == ShortOrganization ||
		t == ShortApplication
}

// IsCollectionType checks if a given URI string matches standard ActivityPub collection types.
func IsCollectionType(uri string) bool {
	return uri == CollectionRegular ||
		uri == CollectionOrdered ||
		uri == CollectionPageRegular ||
		uri == CollectionPageOrdered
}

// IsGroupOrCollectionType checks if a given type string is a Group or any Collection variety exactly.
func IsGroupOrCollectionType(t string) bool {
	return t == ShortGroup ||
		t == ActorGroup ||
		IsCollectionType(t)
}

// IsPrivateCollection checks if a collection short name represents a private, non-public collection.
func IsPrivateCollection(collection string) bool {
	return collection == ShortPendingFollowers ||
		collection == ShortPendingFollowing ||
		collection == ShortBlocked ||
		collection == ShortBlocks
}

// IsCollection checks if a collection short name represents a supported collection in the system.
func IsCollection(collection string) bool {
	return collection == ShortInbox ||
		collection == ShortOutbox ||
		collection == ShortFollowers ||
		collection == ShortFollowing ||
		collection == ShortPendingFollowers ||
		collection == ShortPendingFollowing ||
		collection == ShortBlocked ||
		collection == ShortBlocks ||
		collection == ShortLikes ||
		collection == ShortShares ||
		collection == ShortReplies ||
		collection == ShortContext ||
		collection == ShortContextHistory ||
		collection == ShortFollowersSync
}

// IsCollectionPathSuffix checks if a given URL path segment matches any supported collection path.
func IsCollectionPathSuffix(suffix string) bool {
	return suffix == PathSuffixInbox ||
		suffix == PathSuffixSharedInbox ||
		suffix == PathSuffixOutbox ||
		suffix == PathSuffixFollowers ||
		suffix == PathSuffixFollowing ||
		suffix == PathSuffixPendingFollowers ||
		suffix == PathSuffixPendingFollowing ||
		suffix == PathSuffixBlocked ||
		suffix == PathSuffixBlocks ||
		suffix == PathSuffixLikes ||
		suffix == PathSuffixShares ||
		suffix == PathSuffixReplies ||
		suffix == PathSuffixContext ||
		suffix == PathSuffixContextHistory ||
		suffix == PathSuffixFollowersSync
}

// PathSuffixToCollectionShortName maps a path suffix to the corresponding domain model short name predicate.
func PathSuffixToCollectionShortName(suffix string) string {
	switch suffix {
	case PathSuffixInbox:
		return ShortInbox
	case PathSuffixOutbox:
		return ShortOutbox
	case PathSuffixFollowers:
		return ShortFollowers
	case PathSuffixFollowing:
		return ShortFollowing
	case PathSuffixPendingFollowers:
		return ShortPendingFollowers
	case PathSuffixPendingFollowing:
		return ShortPendingFollowing
	case PathSuffixBlocked:
		return ShortBlocked
	case PathSuffixBlocks:
		return ShortBlocks
	case PathSuffixLikes:
		return ShortLikes
	case PathSuffixShares:
		return ShortShares
	case PathSuffixReplies:
		return ShortReplies
	case PathSuffixContext:
		return ShortContext
	case PathSuffixContextHistory:
		return ShortContextHistory
	case PathSuffixFollowersSync:
		return ShortFollowersSync
	default:
		return ""
	}
}
