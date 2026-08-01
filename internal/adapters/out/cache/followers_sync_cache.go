package cache

import (
	"context"
	"fmt"
	"time"

	"sprezz/internal/domain/port"

	"github.com/dgraph-io/ristretto"
)

type FollowersSyncCacheAdapter struct {
	cache *ristretto.Cache
	ttl   time.Duration
}

func NewFollowersSyncCacheAdapter(ttl time.Duration) (*FollowersSyncCacheAdapter, error) {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	ristrettoCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1_000_000,
		MaxCost:     100_000,
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create followers sync cache: %w", err)
	}

	return &FollowersSyncCacheAdapter{
		cache: ristrettoCache,
		ttl:   ttl,
	}, nil
}

var _ port.FollowersSyncCache = (*FollowersSyncCacheAdapter)(nil)

func (a *FollowersSyncCacheAdapter) GetDigest(ctx context.Context, actorIRI, targetDomain string) (string, bool) {
	key := fmt.Sprintf("%s:%s", actorIRI, targetDomain)
	val, found := a.cache.Get(key)
	if !found || val == nil {
		return "", false
	}
	digest, ok := val.(string)
	return digest, ok
}

func (a *FollowersSyncCacheAdapter) SetDigest(ctx context.Context, actorIRI, targetDomain, digest string) {
	key := fmt.Sprintf("%s:%s", actorIRI, targetDomain)
	a.cache.SetWithTTL(key, digest, 1, a.ttl)
}

func (a *FollowersSyncCacheAdapter) EvictDigest(ctx context.Context, actorIRI, targetDomain string) {
	key := fmt.Sprintf("%s:%s", actorIRI, targetDomain)
	a.cache.Del(key)
}
