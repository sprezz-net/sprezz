package port

import "context"

// FollowersSyncCache defines the pure domain port interface for caching followers collection digests.
type FollowersSyncCache interface {
	GetDigest(ctx context.Context, actorIRI, targetDomain string) (string, bool)
	SetDigest(ctx context.Context, actorIRI, targetDomain, digest string)
	EvictDigest(ctx context.Context, actorIRI, targetDomain string)
}
