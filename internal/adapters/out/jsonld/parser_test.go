package jsonld_test

import (
	"context"
	"strings"
	"testing"

	"sprezz/internal/adapters/out/jsonld"
	"sprezz/internal/domain/model"
)

func TestJSONLDParser_ToQuads(t *testing.T) {
	parser := jsonld.NewJSONLDParser()
	ctx := context.Background()

	// Using the canonical context URL verified by your playground validation run
	jsonPayload := []byte(`{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id": "https://sprezz.net",
		"type": "Note",
		"attributedTo": "https://sprezz.net",
		"content": "Hello ActivityPub Quad Store!",
		"published": "2026-07-24T21:43:00Z"
	}`)

	var targetGraphID int64 = 42
	mainObjectIRI := "https://sprezz.net"

	quads, err := parser.ToQuads(ctx, targetGraphID, mainObjectIRI, jsonPayload)
	if err != nil {
		t.Fatalf("Failed to parse JSON-LD payload to quads: %v", err)
	}

	if len(quads) == 0 {
		t.Fatal("Expected quads from JSON-LD payload, got 0")
	}

	assertGraphInheritance(t, quads, targetGraphID)
	assertSkolemizationIntegrity(t, quads)
	assertSemanticLiterals(t, quads)
}

func assertGraphInheritance(t *testing.T, quads []model.Quad, expectedGraphID int64) {
	for _, q := range quads {
		if q.GraphID != expectedGraphID {
			t.Errorf("Expected GraphID %d, got %d", expectedGraphID, q.GraphID)
		}
	}
}

func assertSkolemizationIntegrity(t *testing.T, quads []model.Quad) {
	for _, q := range quads {
		if strings.HasPrefix(q.Subject, "_:") || strings.HasPrefix(q.Object, "_:") {
			t.Errorf("Leaked unskolemized blank node discovered in quad set: %+v", q)
		}
	}
}

func assertSemanticLiterals(t *testing.T, quads []model.Quad) {
	foundPublished := false
	foundContent := false

	for _, q := range quads {
		if strings.Contains(q.Predicate, "published") {
			foundPublished = true
			validatePublishedField(t, q)
		}
		if q.Object == "Hello ActivityPub Quad Store!" {
			foundContent = true
			validateContentField(t, q)
		}
	}

	if !foundPublished {
		t.Error("Missing expected published timestamp node quad validation context")
	}
	if !foundContent {
		t.Error("Missing expected text payload content node quad validation context")
	}
}

func validatePublishedField(t *testing.T, q model.Quad) {
	if q.ObjType != model.Literal {
		t.Errorf("Expected published property to resolve to model.Literal, got type: %v", q.ObjType)
	}
	expectedLiteralSuffix := `"2026-07-24T21:43:00Z"^^<http://www.w3.org/2001/XMLSchema#dateTime>`
	if q.Object != expectedLiteralSuffix {
		t.Errorf("Datatype literal serialization incorrect.\nWant: %s\nGot:  %s", expectedLiteralSuffix, q.Object)
	}
}

func validateContentField(t *testing.T, q model.Quad) {
	if q.ObjType != model.Literal {
		t.Errorf("Expected content property to resolve to model.Literal, got type: %v", q.ObjType)
	}
}
