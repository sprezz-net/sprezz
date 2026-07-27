package identity_test

import (
	"context"
	"testing"

	"sprezz/internal/domain/identity"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portstub"
)

type mockIdentityStorage struct {
	portstub.UnimplementedStoragePort
	OnGetNomadicIdentity func(ctx context.Context, guid string) (*model.NomadicIdentity, error)
}

func TestIdentityTranslator_InjectNomadicTriples_Success(t *testing.T) {
	// 1. Initialize our clean, isolated translator layer
	storageMock := &mockIdentityStorage{}
	translator := identity.NewIdentityTranslator(storageMock)

	// 2. Setup mock target variables
	var targetGraphID int64 = 42
	actorIRI := "https://sprezz.net"
	guid := "alice-guid-12345"

	// 3. Fire the translation triple generator
	quads, err := translator.InjectNomadicTriples(context.Background(), targetGraphID, actorIRI, guid)
	if err != nil {
		t.Fatalf("Expected flawless triple injection pipeline execution, got error: %v", err)
	}

	// 4. Assert structural lengths match precisely
	if len(quads) != 2 {
		t.Fatalf("Expected exactly 2 nomadic entity quads generated, got %d", len(quads))
	}

	rdfTypeQuad := quads[0]
	if rdfTypeQuad.GraphID != targetGraphID ||
		rdfTypeQuad.Subject != actorIRI ||
		rdfTypeQuad.Predicate != "http://www.w3.org/1999/02/22-rdf-syntax-ns#type" ||
		rdfTypeQuad.Object != "https://www.w3.org/ns/activitystreams#Person" ||
		rdfTypeQuad.ObjType != model.NamedNode {
		t.Errorf("RDF type Quad generation malformed or misaligned: %+v", rdfTypeQuad)
	}

	zotGuidQuad := quads[1]
	expectedObjectLiteral := "alice-guid-12345"
	if zotGuidQuad.GraphID != targetGraphID ||
		zotGuidQuad.Subject != actorIRI ||
		zotGuidQuad.Predicate != "http://purl.org/zot/protocol/guid" ||
		zotGuidQuad.Object != expectedObjectLiteral ||
		zotGuidQuad.ObjType != model.Literal {
		t.Errorf("Zot network identifier mapping tracking Quad malformed: %+v", zotGuidQuad)
	}
}
