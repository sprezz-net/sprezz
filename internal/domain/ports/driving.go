package ports

import (
	"context"
	"sprezz/internal/domain/model"
)

type ActivityServicePort interface {
	ProcessInboundTask(ctx context.Context, task model.InboundTask) error
	DispatchOutboundActivity(ctx context.Context, activityIRI string, actorIRI string, payload []byte) error
	GetFollowersTimeline(ctx context.Context, actorIRI string, limit, offset int) ([]string, error)
}
