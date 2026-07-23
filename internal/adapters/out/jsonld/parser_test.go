package jsonld_test

import (
	"context"
	"testing"

	"sprezz/internal/adapters/out/jsonld"
)

func TestJSONLDParser_ToQuads(t *testing.T) {
	parser := jsonld.NewJSONLDParser()
	ctx := context.Background()

	jsonPayload := []byte(`{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id": "https://sprezz.net/notes/1",
		"type": "Note",
		"attributedTo": "https://sprezz.net/actors/alice",
		"content": "Hello ActivityPub Quad Store!"
	}`)

	quads, err := parser.ToQuads(ctx, 42, "https://sprezz.net/notes/1", jsonPayload)
	if err != nil {
		t.Fatalf("Failed to parse JSON-LD payload to quads: %v", err)
	}

	if len(quads) == 0 {
		t.Fatal("Expected quads from JSON-LD payload, got 0")
	}

	for _, q := range quads {
		if q.GraphID != 42 {
			t.Errorf("Expected GraphID 42, got %d", q.GraphID)
		}
	}
}
