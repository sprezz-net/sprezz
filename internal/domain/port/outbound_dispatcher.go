package port

import (
	"context"
)

// OutboundDispatcher isolates the network egress and signature logic required for federation.
// It handles secure HTTP transport delivery to foreign ActivityPub and Nomad inbox channels.
type OutboundDispatcher interface {
	// ForwardFederatedActivity executes a signed HTTP POST request pushing an activity payload
	// out to a targeted external inbox. It signs the request using the provided dual-key pairs.
	ForwardFederatedActivity(
		ctx context.Context,
		targetInbox string,
		actorKeyID string,
		privateKeyRSAPEM string,
		privateKeyEd25519PEM string,
		payload []byte,
	) error
}
