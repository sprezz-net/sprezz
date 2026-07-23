package cache

import (
	"fmt"
	"github.com/dgraph-io/ristretto"
)

type DictionaryCache struct {
	uriToID *ristretto.Cache
	idToURI *ristretto.Cache
}

func NewDictionaryCache() (*DictionaryCache, error) {
	uriToIDCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 10_000_000,
		MaxCost:     1_000_000,
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create uriToID cache: %w", err)
	}

	idToURICache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 10_000_000,
		MaxCost:     1_000_000,
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create idToURI cache: %w", err)
	}

	return &DictionaryCache{
		uriToID: uriToIDCache,
		idToURI: idToURICache,
	}, nil
}

func (c *DictionaryCache) GetID(uri string) (int64, bool) {
	val, found := c.uriToID.Get(uri)
	if !found || val == nil {
		return 0, false
	}
	id, ok := val.(int64)
	return id, ok
}

func (c *DictionaryCache) GetURI(id int64) (string, bool) {
	val, found := c.idToURI.Get(id)
	if !found || val == nil {
		return "", false
	}
	uri, ok := val.(string)
	return uri, ok
}

func (c *DictionaryCache) Set(uri string, id int64) {
	c.uriToID.Set(uri, id, 1)
	c.idToURI.Set(id, uri, 1)
	c.uriToID.Wait()
	c.idToURI.Wait()
}
