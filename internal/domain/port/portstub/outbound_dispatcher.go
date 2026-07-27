package portstub

import (
	"context"

	"sprezz/internal/domain/port"
)

type UnimplementedOutboundDispatcher struct{}

var _ port.OutboundDispatcher = (*UnimplementedOutboundDispatcher)(nil)

func (UnimplementedOutboundDispatcher) ForwardFederatedActivity(
	ctx context.Context,
	targetInbox string,
	actorKeyID string,
	privateKeyRSAPEM string,
	privateKeyEd25519PEM string,
	payload []byte,
) error {
	return nil
}
