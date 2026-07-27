package portstub

import (
	"context"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
)

type UnimplementedActivityServicePort struct{}

var _ port.ActivityServicePort = (*UnimplementedActivityServicePort)(nil)

func (UnimplementedActivityServicePort) ProcessInboundTask(ctx context.Context, task model.InboundTask) error {
	return nil

}

func (UnimplementedActivityServicePort) DispatchOutboundActivity(ctx context.Context, activityIRI string, actorIRI string, payload []byte) error {
	return nil

}

func (UnimplementedActivityServicePort) GetFollowersTimeline(ctx context.Context, actorIRI string, limit, offset int) ([]string, error) {
	return nil, nil
}
