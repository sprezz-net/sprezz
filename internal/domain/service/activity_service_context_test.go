package service

import (
	"context"
	"errors"
	"testing"

	"github.com/gojuno/minimock/v3"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"
)

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

	found := false
	for _, q := range res {
		if q.Subject == "https://local.com/objects/note-123" && q.Predicate == model.PredicateContext {
			found = true
			if q.Object != "https://local.com/objects/note-123/context" {
				t.Errorf("expected context IRI to be 'https://local.com/objects/note-123/context', got: %s", q.Object)
			}
		}
	}
	if !found {
		t.Error("expected context quad to be generated")
	}
}

func TestEnsureContextRelation_Reply(t *testing.T) {
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStoragePortMock(mc)
	mockStorage.StreamQuadsBySubjectMock.Set(func(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
		if subjectIRI == "https://remote.com/objects/note-999" {
			return []model.Quad{
				{Subject: "https://remote.com/objects/note-999", Predicate: model.PredicateContext, Object: "https://remote.com/objects/note-999/context", ObjType: model.NamedNode},
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

	found := false
	for _, q := range res {
		if q.Subject == "https://local.com/objects/reply-456" && q.Predicate == model.PredicateContext {
			found = true
			if q.Object != "https://remote.com/objects/note-999/context" {
				t.Errorf("expected context IRI to be inherited as 'https://remote.com/objects/note-999/context', got: %s", q.Object)
			}
		}
	}
	if !found {
		t.Error("expected context quad to be inherited")
	}
}
