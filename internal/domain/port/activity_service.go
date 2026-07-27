package port

import (
	"context"
	"io"
	"sprezz/internal/domain/model"
)

// InboundMediaContext bundles metadata context for incoming loop iterations.
type InboundMediaContext struct {
	TenantID     string
	ActorIRI     string
	ObjectName   string
	OriginalName string
	ContentType  string
	Size         int64
	MediaStream  io.Reader
}

// ActivityServicePort presents the driving use-case boundary for ActivityPub activities.
type ActivityServicePort interface {
	ProcessInboundTask(ctx context.Context, task model.InboundTask) error
	ProcessInboundMediaTask(ctx context.Context, mediaCtx InboundMediaContext, task model.InboundTask) error
	PurgeOrphanedMedia(ctx context.Context, tempObjectKey string) error
	DispatchOutboundActivity(ctx context.Context, activityIRI string, actorIRI string, payload []byte) error
	GetFollowersTimeline(ctx context.Context, actorIRI string, limit, offset int) ([]string, error)
}
