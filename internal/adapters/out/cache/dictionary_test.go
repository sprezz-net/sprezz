package cache_test

import (
	"testing"
	"time"

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

	// Set URI <-> ID mapping asynchronously
	dictCache.Set(uri, id)

	// FIXED: Add a small retry backoff window to accommodate Ristretto's
	// internal asynchronous batch ring buffer assignment delays without breaking execution.
	var gotID int64
	var foundID bool
	for i := 0; i < 10; i++ {
		gotID, foundID = dictCache.GetID(uri)
		if foundID {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !foundID || gotID != id {
		t.Errorf("GetID(%s) = (%d, %v), want (%d, true)", uri, gotID, foundID, id)
	}

	gotURI, foundURI := dictCache.GetURI(id)
	if !foundURI || gotURI != uri {
		t.Errorf("GetURI(%d) = (%s, %v), want (%s, true)", id, gotURI, foundURI, uri)
	}
}

func TestDictionaryCache_Clear(t *testing.T) {
	dictCache, err := cache.NewDictionaryCache()
	if err != nil {
		t.Fatalf("Failed to create DictionaryCache: %v", err)
	}

	uri := "https://w3.org"
	id := int64(2002)

	dictCache.Set(uri, id)

	// Wait for async ingestion to settle down
	for i := 0; i < 10; i++ {
		if _, found := dictCache.GetID(uri); found {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Execute clear context command loop mapping
	dictCache.Clear()

	// FIXED: Verify that clear explicitly empties out both internal indexes cleanly
	if _, found := dictCache.GetID(uri); found {
		t.Error("Expected cache miss for URI after executing Clear()")
	}
	if _, found := dictCache.GetURI(id); found {
		t.Error("Expected cache miss for ID after executing Clear()")
	}
}
