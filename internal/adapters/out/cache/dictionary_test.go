package cache_test

import (
	"testing"

	"sprezz/internal/adapters/out/cache"
)

func TestDictionaryCache_GetAndSet(t *testing.T) {
	dictCache, err := cache.NewDictionaryCache()
	if err != nil {
		t.Fatalf("Failed to create DictionaryCache: %v", err)
	}

	uri := "https://www.w3.org/ns/activitystreams#Note"
	id := int64(1001)

	// Verify miss before set
	if _, found := dictCache.GetID(uri); found {
		t.Error("Expected cache miss for URI before setting")
	}
	if _, found := dictCache.GetURI(id); found {
		t.Error("Expected cache miss for ID before setting")
	}

	// Set URI <-> ID mapping
	dictCache.Set(uri, id)

	// Verify hits after set
	gotID, foundID := dictCache.GetID(uri)
	if !foundID || gotID != id {
		t.Errorf("GetID(%s) = (%d, %v), want (%d, true)", uri, gotID, foundID, id)
	}

	gotURI, foundURI := dictCache.GetURI(id)
	if !foundURI || gotURI != uri {
		t.Errorf("GetURI(%d) = (%s, %v), want (%s, true)", id, gotURI, foundURI, uri)
	}
}
