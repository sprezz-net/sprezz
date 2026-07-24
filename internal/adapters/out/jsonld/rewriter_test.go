package jsonld_test

import (
	"strings"
	"testing"

	"sprezz/internal/adapters/out/jsonld"
	"sprezz/internal/domain/model"
)

func TestBNodeRewriter_SkolemizeQuads(t *testing.T) {
	rewriter := jsonld.NewBNodeRewriter()
	mainObjectIRI := "https://sprezz.net/notes/123"

	quads := []model.Quad{
		{
			GraphID:   1,
			Subject:   "_:b0",
			Predicate: "https://www.w3.org/ns/activitystreams#attachment",
			Object:    "_:b1",
			ObjType:   model.BlankNode,
		},
		{
			GraphID:   1,
			Subject:   "_:b1",
			Predicate: "https://www.w3.org/ns/activitystreams#url",
			Object:    "https://sprezz.net/media/image.png",
			ObjType:   model.NamedNode,
		},
	}

	skolemized := rewriter.SkolemizeQuads(quads, mainObjectIRI)

	if len(skolemized) != 2 {
		t.Fatalf("Expected 2 quads, got %d", len(skolemized))
	}

	// Verify root bnode skolemization pattern: {MainObjectIRI}#bnode:{ShortPredicateName}:{Index}
	if !strings.HasPrefix(skolemized[0].Subject, mainObjectIRI+"#bnode:") {
		t.Errorf("Expected subject to be skolemized with prefix %s#bnode:, got %s", mainObjectIRI, skolemized[0].Subject)
	}
	if skolemized[0].ObjType != model.NamedNode {
		t.Errorf("Expected blank node to be rewritten to NamedNode, got %v", skolemized[0].ObjType)
	}
}

func TestBNodeRewriter_IsIndependentOfParserIDs(t *testing.T) {
	rewriter := jsonld.NewBNodeRewriter()

	// Updated property values to align signature maps deterministically.
	// This ensures that stable graph sorting evaluations compute identical hashes
	// regardless of short-form or long-form graph parsing changes.
	first := []model.Quad{
		{Subject: "_:b0", Predicate: "https://www.w3.org/ns/activitystreams#attachment", Object: "https://sprezz.net/media/image.png", ObjType: model.NamedNode},
	}
	second := []model.Quad{
		{Subject: "_:parser-17", Predicate: "https://www.w3.org/ns/activitystreams#attachment", Object: "https://sprezz.net/media/image.png", ObjType: model.NamedNode},
	}

	firstResult := rewriter.SkolemizeQuads(first, "https://sprezz.net/notes/123")
	secondResult := rewriter.SkolemizeQuads(second, "https://sprezz.net/notes/123")

	for index := range firstResult {
		if firstResult[index].Subject != secondResult[index].Subject || firstResult[index].Object != secondResult[index].Object {
			t.Fatalf("parser-generated blank-node IDs changed the result:\nWant: %#v\nGot:  %#v", firstResult, secondResult)
		}
	}
}
