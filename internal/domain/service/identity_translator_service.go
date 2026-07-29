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
			Predicate: model.RDFType,
			Object:    model.ActorPerson,
			ObjType:   model.NamedNode,
		},
		{
			GraphID:   graphID,
			Subject:   actorIRI,
			Predicate: model.PredicateZotGUID,
			Object:    guid,
			ObjType:   model.Literal,
		},
	}, nil
}
