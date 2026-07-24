package identity_test

import (
	"context"
	"testing"

	"sprezz/internal/adapters/out/identity"
	"sprezz/internal/domain/model"
)

// Minimal mock storage to satisfy the initialization constraints of the translator
type mockIdentityStorage struct{}

func (m *mockIdentityStorage) IsDomainBlocked(ctx context.Context, d string) (bool, error) { return false, nil }
func (m *mockIdentityStorage) EnqueueInbound(ctx context.Context, id, a, o, t string, p []byte) error {
	return nil
}
func (m *mockIdentityStorage) ClaimInboundBatch(ctx context.Context, b int) ([]model.InboundTask, error) {
	return nil, nil
}
func (m *mockIdentityStorage) MarkInboundComplete(ctx context.Context, id string) error  { return nil }
func (m *mockIdentityStorage) MarkInboundFailed(ctx context.Context, id, r string) error { return nil }
func (m *mockIdentityStorage) GetNomadicIdentity(ctx context.Context, g string) (*model.NomadicIdentity, error) {
	return nil, nil
}
func (m *mockIdentityStorage) UpsertNomadicIdentity(ctx context.Context, i *model.NomadicIdentity) error {
	return nil
}
func (m *mockIdentityStorage) RegisterIdentityClone(ctx context.Context, g, h string, l bool) error {
	return nil
}
func (m *mockIdentityStorage) GetActorPrivateKey(ctx context.Context, a string) (string, error) {
	return "", nil
}
func (m *mockIdentityStorage) CreateGraphVersion(ctx context.Context, a, o string, p []byte) (int64, error) {
	return 0, nil
}
func (m *mockIdentityStorage) SaveQuads(ctx context.Context, q []model.Quad) error      { return nil }
func (m *mockIdentityStorage) RemoveQuadEdge(ctx context.Context, s, p, o string) error { return nil }
func (m *mockIdentityStorage) GetLatestPayload(ctx context.Context, o string) ([]byte, error) {
	return nil, nil
}
func (m *mockIdentityStorage) StreamQuadsBySubject(ctx context.Context, s string) ([]model.Quad, error) {
	return nil, nil
}
func (m *mockIdentityStorage) GetCollectionPayloads(ctx context.Context, a, c string, l, o int) ([][]byte, error) {
	return nil, nil
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

	// 5. FIXED: Added explicit index slice subscripts [0] to extract the element correctly
	rdfTypeQuad := quads[0]
	if rdfTypeQuad.GraphID != targetGraphID ||
		rdfTypeQuad.Subject != actorIRI ||
		rdfTypeQuad.Predicate != "http://w3.org" ||
		rdfTypeQuad.Object != "https://w3.org" ||
		rdfTypeQuad.ObjType != model.NamedNode {
		t.Errorf("RDF type Quad generation malformed or misaligned: %+v", rdfTypeQuad)
	}

	// 6. FIXED: Added explicit index slice subscripts [1] to extract the element correctly
	zotGuidQuad := quads[1]
	if zotGuidQuad.GraphID != targetGraphID ||
		zotGuidQuad.Subject != actorIRI ||
		zotGuidQuad.Predicate != "http://purl.org" ||
		zotGuidQuad.Object != guid ||
		zotGuidQuad.ObjType != model.Literal {
		t.Errorf("Zot network identifier mapping tracking Quad malformed: %+v", zotGuidQuad)
	}
}
