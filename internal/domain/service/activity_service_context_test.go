package service

import (
	"context"
	"errors"
	"testing"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"

	"github.com/gojuno/minimock/v3"
)

func verifyQuad(t *testing.T, res []model.Quad, subject, predicate, expectedObject string) bool {
	for _, q := range res {
		if q.Subject == subject && q.Predicate == predicate {
			if q.Object != expectedObject {
				t.Errorf("expected %s IRI to be '%s', got: %s", predicate, expectedObject, q.Object)
			}
			return true
		}
	}
	return false
}

func TestEnsureContextRelation_TopLevel(t *testing.T) {
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStoragePortMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockFetcher := portmock.NewRemoteFetcherMock(mc)

	svc := NewActivityService(mockStorage, mockParser, mockMedia, mockFetcher, ActivityServiceConfig{})

	quads := []model.Quad{
		{Subject: "https://local.com/objects/note-123", Predicate: model.RDFType, Object: model.TypeNote, ObjType: model.NamedNode},
	}

	res, err := svc.ensureContextRelation(context.Background(), "https://local.com/objects/note-123", quads)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := verifyQuad(t, res, "https://local.com/objects/note-123", model.PredicateContext, "https://local.com/objects/note-123/context")
	foundHistory := verifyQuad(t, res, "https://local.com/objects/note-123", model.PredicateContextHistory, "https://local.com/objects/note-123/contextHistory")

	if !found {
		t.Error("expected context quad to be generated")
	}
	if !foundHistory {
		t.Error("expected contextHistory quad to be generated")
	}
}

func TestEnsureContextRelation_Reply(t *testing.T) {
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStoragePortMock(mc)
	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/objects/note-999" {
			return []model.Quad{
				{Subject: "https://remote.com/objects/note-999", Predicate: model.PredicateContext, Object: "https://remote.com/objects/note-999/context", ObjType: model.NamedNode},
				{Subject: "https://remote.com/objects/note-999", Predicate: model.PredicateContextHistory, Object: "https://remote.com/objects/note-999/contextHistory", ObjType: model.NamedNode},
			}, nil
		}
		return nil, errors.New("not found")
	})

	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockFetcher := portmock.NewRemoteFetcherMock(mc)

	svc := NewActivityService(mockStorage, mockParser, mockMedia, mockFetcher, ActivityServiceConfig{})

	quads := []model.Quad{
		{Subject: "https://local.com/objects/reply-456", Predicate: model.RDFType, Object: model.TypeNote, ObjType: model.NamedNode},
		{Subject: "https://local.com/objects/reply-456", Predicate: model.NamespaceActivityStreams + "inReplyTo", Object: "https://remote.com/objects/note-999", ObjType: model.NamedNode},
	}

	res, err := svc.ensureContextRelation(context.Background(), "https://local.com/objects/reply-456", quads)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := verifyQuad(t, res, "https://local.com/objects/reply-456", model.PredicateContext, "https://remote.com/objects/note-999/context")
	foundHistory := verifyQuad(t, res, "https://local.com/objects/reply-456", model.PredicateContextHistory, "https://remote.com/objects/note-999/contextHistory")

	if !found {
		t.Error("expected context quad to be inherited")
	}
	if !foundHistory {
		t.Error("expected contextHistory quad to be inherited")
	}
}
