package jsonld_test

import (
	"testing"

	"sprezz/internal/adapters/out/jsonld"
)

func TestEmbeddedDocumentLoader_LoadDocument(t *testing.T) {
	loader := jsonld.NewEmbeddedDocumentLoader()

	// 1. Load embedded ActivityStreams context
	docAS, err := loader.LoadDocument("https://www.w3.org/ns/activitystreams")
	if err != nil {
		t.Fatalf("Failed to load embedded ActivityStreams context: %v", err)
	}
	if docAS == nil || docAS.Document == nil {
		t.Fatal("Loaded document is nil")
	}

	// 2. Load embedded Security v1 context
	docSec, err := loader.LoadDocument("https://w3id.org/security/v1")
	if err != nil {
		t.Fatalf("Failed to load embedded Security v1 context: %v", err)
	}
	if docSec == nil || docSec.Document == nil {
		t.Fatal("Loaded document is nil")
	}

	// 3. SSRF Defense: Remote unknown URLs should be blocked
	_, errBlocked := loader.LoadDocument("https://malicious-external-site.com/context.json")
	if errBlocked == nil {
		t.Error("Expected offline document loader to block unknown remote URL, but it succeeded")
	}
}
