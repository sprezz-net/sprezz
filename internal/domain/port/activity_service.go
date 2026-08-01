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

// MediaAttachmentInfo bundles calculated metadata and FEP-1311 properties returned from ingestion.
type MediaAttachmentInfo struct {
	ID              string
	ObjectName      string
	OriginalName    string
	SHA256Hex       string
	DigestMultibase string
	ContentType     string
	Size            int64
	Width           int
	Height          int
}

// ActivityServicePort presents the driving use-case boundary for ActivityPub activities.
type ActivityServicePort interface {
	ProcessInboundTask(ctx context.Context, task model.InboundTask) error
	ProcessInboundMediaTask(ctx context.Context, mediaCtx InboundMediaContext, task model.InboundTask) (MediaAttachmentInfo, error)
	PurgeOrphanedMedia(ctx context.Context, tempObjectKey string) error
	DispatchOutboundActivity(ctx context.Context, activityIRI string, actorIRI string, payload []byte) error
	GetFollowersTimeline(ctx context.Context, actorIRI string, limit, offset int) ([]string, error)
	GetCollectionTimeline(ctx context.Context, readerActorIRI string, actorIRI string, collection string, limit, offset int) ([][]byte, error)
	AcceptFollow(ctx context.Context, followedActorIRI, followActivityIRI string) error
	RejectFollow(ctx context.Context, followedActorIRI, followActivityIRI string) error
	SyncFollowers(ctx context.Context, actorIRI, remoteCollectionID, remoteSyncURL, expectedDigest string) error
}
