package postgres

import (
	"context"
	"errors"
	"fmt"

	"sprezz/internal/adapters/out/cache"
	"sprezz/internal/adapters/out/postgres/db"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStorage struct {
	db    *pgxpool.Pool
	cache *cache.DictionaryCache
}

func NewPostgresStorage(db *pgxpool.Pool, cache *cache.DictionaryCache) *PostgresStorage {
	return &PostgresStorage{db: db, cache: cache}
}

var _ ports.StoragePort = (*PostgresStorage)(nil)
var _ ports.GraphVersionWriter = (*PostgresStorage)(nil)

func (s *PostgresStorage) queries() *db.Queries { return db.New(s.db) }

func (s *PostgresStorage) IsDomainBlocked(ctx context.Context, domainName string) (bool, error) {
	return s.queries().IsDomainBlocked(ctx, domainName)
}

func (s *PostgresStorage) EnqueueInbound(ctx context.Context, id string, activityIRI, objectIRI, targetDomain string, payload []byte) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	queries := db.New(tx)
	if err := queries.InsertTenant(ctx, targetDomain); err != nil {
		return err
	}
	tenantID, err := queries.GetTenantID(ctx, targetDomain)
	if err != nil {
		return fmt.Errorf("resolve tenant %q: %w", targetDomain, err)
	}
	queueID, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("parse inbound queue ID: %w", err)
	}
	if err := queries.EnqueueInboundActivity(ctx, db.EnqueueInboundActivityParams{ID: queueID, ActivityIri: activityIRI, ObjectIri: objectIRI, Payload: payload}); err != nil {
		return err
	}
	if err := queries.RecordTenantDelivery(ctx, db.RecordTenantDeliveryParams{ActivityIri: activityIRI, TenantID: tenantID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStorage) RecordActorInboxDelivery(ctx context.Context, actorIRI, activityIRI string) error {
	return s.queries().RecordActorInboxDelivery(ctx, db.RecordActorInboxDeliveryParams{ActorIri: actorIRI, ActivityIri: activityIRI})
}

func (s *PostgresStorage) GetCollectionPayloads(ctx context.Context, actorIRI, collection string, limit, offset int) ([][]byte, error) {
	queries := s.queries()
	switch collection {
	case "inbox":
		return queries.GetInboxPayloads(ctx, db.GetInboxPayloadsParams{ActorIri: actorIRI, Limit: int32(limit), Offset: int32(offset)})
	case "outbox":
		return queries.GetOutboxPayloads(ctx, db.GetOutboxPayloadsParams{ActorIri: actorIRI, Limit: int32(limit), Offset: int32(offset)})
	default:
		return nil, fmt.Errorf("unsupported collection %q", collection)
	}
}

func (s *PostgresStorage) ClaimInboundBatch(ctx context.Context, batchSize int) ([]model.InboundTask, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	queries := db.New(tx)
	rows, err := queries.ClaimInboundTasks(ctx, int32(batchSize))
	if err != nil {
		return nil, err
	}
	ids := make([]pgtype.UUID, 0, len(rows))
	tasks := make([]model.InboundTask, 0, len(rows))
	for _, row := range rows {
		id, err := uuidFromPG(row.ID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, row.ID)
		tasks = append(tasks, model.InboundTask{ID: id.String(), ActivityIRI: row.ActivityIri, ObjectIRI: row.ObjectIri, Payload: row.Payload})
	}
	if len(ids) > 0 {
		if err := queries.MarkInboundProcessing(ctx, ids); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *PostgresStorage) MarkInboundComplete(ctx context.Context, id string) error {
	queueID, err := parseUUID(id)
	if err != nil {
		return err
	}
	return s.queries().MarkInboundComplete(ctx, queueID)
}

func (s *PostgresStorage) MarkInboundFailed(ctx context.Context, id string, reason string) error {
	queueID, err := parseUUID(id)
	if err != nil {
		return err
	}
	return s.queries().MarkInboundFailed(ctx, db.MarkInboundFailedParams{ID: queueID, ErrorMessage: pgtype.Text{String: reason, Valid: true}})
}

func (s *PostgresStorage) GetNomadicIdentity(ctx context.Context, guid string) (*model.NomadicIdentity, error) {
	identity, err := s.queries().GetNomadicIdentity(ctx, guid)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	hubs, err := s.queries().GetIdentityCloneHubs(ctx, guid)
	if err != nil {
		return nil, err
	}
	return &model.NomadicIdentity{GUID: identity.Guid, PrimaryHubURL: identity.PrimaryHubUrl, MasterPublicKeyPEM: identity.MasterPublicKeyPem, ClonedHubs: hubs}, nil
}

func (s *PostgresStorage) UpsertNomadicIdentity(ctx context.Context, identity *model.NomadicIdentity) error {
	return s.queries().UpsertNomadicIdentity(ctx, db.UpsertNomadicIdentityParams{Guid: identity.GUID, PrimaryHubUrl: identity.PrimaryHubURL, MasterPublicKeyPem: identity.MasterPublicKeyPEM})
}

func (s *PostgresStorage) RegisterIdentityClone(ctx context.Context, guid, hubURL string, isLocal bool) error {
	return s.queries().RegisterIdentityClone(ctx, db.RegisterIdentityCloneParams{IdentityGuid: guid, HubUrl: hubURL, IsLocal: pgtype.Bool{Bool: isLocal, Valid: true}})
}

func (s *PostgresStorage) GetActorPrivateKey(ctx context.Context, actorIRI string) (string, error) {
	return s.queries().GetActorPrivateKey(ctx, actorIRI)
}

func (s *PostgresStorage) CreateGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte) (int64, error) {
	return s.queries().CreateGraphVersion(ctx, db.CreateGraphVersionParams{ActivityID: activityIRI, ObjectIri: objectIRI, Payload: payload})
}

func (s *PostgresStorage) SaveGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	queries := db.New(tx)
	graphID, err := queries.CreateGraphVersion(ctx, db.CreateGraphVersionParams{ActivityID: activityIRI, ObjectIri: objectIRI, Payload: payload})
	if err != nil {
		return err
	}
	if err := s.saveQuads(ctx, queries, graphID, quads); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStorage) SaveQuads(ctx context.Context, quads []model.Quad) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err := s.saveQuads(ctx, db.New(tx), 0, quads); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStorage) saveQuads(ctx context.Context, queries *db.Queries, graphID int64, quads []model.Quad) error {
	for _, quad := range quads {
		if graphID != 0 {
			quad.GraphID = graphID
		}
		subjectID, err := s.dictionaryID(ctx, queries, quad.Subject)
		if err != nil {
			return err
		}
		predicateID, err := s.dictionaryID(ctx, queries, quad.Predicate)
		if err != nil {
			return err
		}
		objectID, err := s.dictionaryID(ctx, queries, quad.Object)
		if err != nil {
			return err
		}
		if err := queries.InsertQuad(ctx, db.InsertQuadParams{GraphID: quad.GraphID, SubjectID: subjectID, PredicateID: predicateID, ObjectID: objectID, IsLiteral: pgtype.Bool{Bool: quad.IsLiteral(), Valid: true}}); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStorage) RemoveQuadEdge(ctx context.Context, subject, predicate, object string) error {
	ids := []int64{}
	for _, value := range []string{subject, predicate, object} {
		id, found := s.cache.GetID(value)
		if !found {
			var err error
			id, err = s.queries().GetDictionaryID(ctx, value)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			s.cache.Set(value, id)
		}
		ids = append(ids, id)
	}
	return s.queries().RemoveQuadEdge(ctx, db.RemoveQuadEdgeParams{SubjectID: ids[0], PredicateID: ids[1], ObjectID: ids[2]})
}

func (s *PostgresStorage) GetLatestPayload(ctx context.Context, objectIRI string) ([]byte, error) {
	payload, err := s.queries().GetLatestPayload(ctx, objectIRI)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return payload, err
}

func (s *PostgresStorage) StreamQuadsBySubject(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
	subjectID, found := s.cache.GetID(subjectIRI)
	if !found {
		var err error
		subjectID, err = s.queries().GetDictionaryID(ctx, subjectIRI)
		if errors.Is(err, pgx.ErrNoRows) {
			return []model.Quad{}, nil
		}
		if err != nil {
			return nil, err
		}
		s.cache.Set(subjectIRI, subjectID)
	}
	rows, err := s.queries().GetSubjectQuads(ctx, subjectID)
	if err != nil {
		return nil, err
	}
	quads := make([]model.Quad, 0, len(rows))
	for _, row := range rows {
		objType := model.NamedNode
		if row.IsLiteral.Valid && row.IsLiteral.Bool {
			objType = model.Literal
		}
		quads = append(quads, model.Quad{GraphID: row.GraphID, Subject: subjectIRI, Predicate: row.Predicate, Object: row.Object, ObjType: objType})
	}
	return quads, nil
}

func (s *PostgresStorage) dictionaryID(ctx context.Context, queries *db.Queries, value string) (int64, error) {
	if id, found := s.cache.GetID(value); found {
		return id, nil
	}
	id, err := queries.InsertDictionaryValue(ctx, value)
	if err == nil {
		s.cache.Set(value, id)
	}
	return id, err
}

func parseUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func uuidFromPG(value pgtype.UUID) (uuid.UUID, error) {
	if !value.Valid {
		return uuid.Nil, fmt.Errorf("invalid database UUID")
	}
	return uuid.UUID(value.Bytes), nil
}

func rollback(tx pgx.Tx) {
	_ = tx.Rollback(context.Background())
}
