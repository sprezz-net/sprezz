package jsonld_test

import (
	"errors"
	"testing"

	"sprezz/internal/adapters/out/jsonld"
	"github.com/piprate/json-gold/ld"
)

// mockFallbackLoader intercepts requests routed outside local memory spaces
type mockFallbackLoader struct {
	calledWithURL string
	mockErr       error
}

func (m *mockFallbackLoader) LoadDocument(u string) (*ld.RemoteDocument, error) {
	m.calledWithURL = u
	if m.mockErr != nil {
		return nil, m.mockErr
	}
	return &ld.RemoteDocument{DocumentURL: u, Document: map[string]interface{}{}}, nil
}

func TestEmbeddedDocumentLoader_LoadDocument_EmbeddedHits(t *testing.T) {
	// Use standard constructor for verifying local embedded caching performance
	loader := jsonld.NewEmbeddedDocumentLoader()

	// 1. Load embedded ActivityStreams context from memory FS
	docAS, err := loader.LoadDocument("https://www.w3.org/ns/activitystreams")
	if err != nil {
		t.Fatalf("Failed to load embedded ActivityStreams context: %v", err)
	}
	if docAS == nil || docAS.Document == nil {
		t.Fatal("Loaded ActivityStreams document or inner payload reference is nil")
	}

	// 2. Load embedded Security v1 context from memory FS
	docSec, err := loader.LoadDocument("https://w3id.org/security/v1")
	if err != nil {
		t.Fatalf("Failed to load embedded Security v1 context: %v", err)
	}
	if docSec == nil || docSec.Document == nil {
		t.Fatal("Loaded Security v1 document or inner payload reference is nil")
	}
}

func TestEmbeddedDocumentLoader_LoadDocument_FallbackRouting(t *testing.T) {
	mockFallback := &mockFallbackLoader{
		mockErr: errors.New("mock network timeout or blocked path simulation"),
	}

	// Inject our custom network mock observer
	loader := jsonld.NewEmbeddedDocumentLoaderWithFallback(mockFallback)
	targetExternalURL := "https://joinmastodon.org"

	// 3. Verify that external or platform-specific extension context URLs route
	// directly to the fallback network handler without running local FS checks
	_, err := loader.LoadDocument(targetExternalURL)

	if mockFallback.calledWithURL != targetExternalURL {
		t.Errorf("Expected external extension request to route to fallback handler for URL %q, but handler was never notified", targetExternalURL)
	}

	if err == nil || err.Error() != "mock network timeout or blocked path simulation" {
		t.Errorf("Expected payload delivery to return the bubble-up exception from the fallback layer, got: %v", err)
	}
}
