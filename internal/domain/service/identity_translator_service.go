package service

import (
	"context"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
)

type IdentityTranslatorService struct {
	storage port.StoragePort
}

func NewIdentityTranslatorService(storage port.StoragePort) *IdentityTranslatorService {
	return &IdentityTranslatorService{storage: storage}
}

func (t *IdentityTranslatorService) InjectNomadicTriples(ctx context.Context, graphID int64, actorIRI string, guid string) ([]model.Quad, error) {
	return []model.Quad{
		{
			GraphID:   graphID,
			Subject:   actorIRI,
			Predicate: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
			Object:    "https://www.w3.org/ns/activitystreams#Person",
			ObjType:   model.NamedNode,
		},
		{
			GraphID:   graphID,
			Subject:   actorIRI,
			Predicate: "http://purl.org/zot/protocol/guid",
			Object:    guid,
			ObjType:   model.Literal,
		},
	}, nil
}
