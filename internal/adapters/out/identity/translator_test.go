package identity_test

import (
	"context"
	"testing"

	"sprezz/internal/adapters/out/identity"
	"sprezz/internal/domain/model"
)

func TestIdentityTranslator_InjectNomadicTriples(t *testing.T) {
	translator := identity.NewIdentityTranslator(nil)
	ctx := context.Background()

	graphID := int64(10)
	actorIRI := "https://sprezz.net/actors/alice"
	guid := "alice-guid-12345"

	quads, err := translator.InjectNomadicTriples(ctx, graphID, actorIRI, guid)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(quads) != 2 {
		t.Fatalf("Expected 2 quads, got %d", len(quads))
	}

	if quads[0].Subject != actorIRI || quads[0].Object != "https://www.w3.org/ns/activitystreams#Person" {
		t.Errorf("Unexpected type quad: %+v", quads[0])
	}

	if quads[1].Subject != actorIRI || quads[1].Object != guid || quads[1].ObjType != model.Literal {
		t.Errorf("Unexpected GUID quad: %+v", quads[1])
	}
}
