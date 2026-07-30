package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sprezz/internal/adapters/out/cache"
	"sprezz/internal/adapters/out/postgres/db"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"

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

var _ port.StoragePort = (*PostgresStorage)(nil)
var _ port.GraphVersionWriter = (*PostgresStorage)(nil)

func (s *PostgresStorage) queries() *db.Queries { return db.New(s.db) }

// Multi-Tenant Bootstrap and Pre-Flight Storage Metric Hooks

func (s *PostgresStorage) UpsertConfiguredTenant(ctx context.Context, tenantUUID string, domainName string) (int32, error) {
	uuidVal, err := parseUUID(tenantUUID)
	if err != nil {
		return 0, fmt.Errorf("invalid tenant UUID format: %w", err)
	}

	newTenant, err := s.queries().InsertTenant(ctx, db.InsertTenantParams{
		TenantUuid: uuidVal,
		DomainName: domainName,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to upsert configured tenant boundary for %s: %w", domainName, err)
	}
	return newTenant.ID, nil
}

func (s *PostgresStorage) GetAllTenants(ctx context.Context) (map[string]int32, error) {
	rows, err := s.queries().GetAllTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving all tenants: %w", err)
	}

	tenants := make(map[string]int32, len(rows))
	for _, r := range rows {
		tenants[r.DomainName] = r.ID
	}
	return tenants, nil
}

func (s *PostgresStorage) GetTenantIDByDomain(ctx context.Context, domainName string) (int32, error) {
	row, err := s.queries().GetTenantByDomain(ctx, domainName)
	if err != nil {
		return 0, fmt.Errorf("failed looking up tenant partition details: %w", err)
	}
	return row.ID, nil
}

func (s *PostgresStorage) HasActorCredential(ctx context.Context, tenantID int32, username string) (bool, error) {
	// Check for database presence using the clean credentials filter query [source: 3]
	_, err := s.queries().GetActorCredentialsByUsername(ctx, db.GetActorCredentialsByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed checking database actor registration availability: %w", err)
	}
	return true, nil
}

func (s *PostgresStorage) GetActorCredentials(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error) {
	// 1. Execute the type-safe multi-column selection query from your tenants.sql [source: 3]
	row, err := s.queries().GetActorCredentialsByUsername(ctx, db.GetActorCredentialsByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed looking up dual-key credentials row: %w", err)
	}

	// 2. Return the stable actor_iri and map variables securely to the domain structure.
	var privKeyEd25519 string
	if row.PrivateKeyEd25519Pem != nil {
		privKeyEd25519 = *row.PrivateKeyEd25519Pem
	}
	return row.ActorIri, &model.ActorDualKeys{
		PrivateKeyRSAPEM:     row.PrivateKeyRsaPem,
		PrivateKeyEd25519PEM: privKeyEd25519,
	}, nil
}

func (s *PostgresStorage) CreateActorCredential(ctx context.Context, actorIRI string, tenantID int32, username string, privateKeyRSAPEM string, privateKeyEd25519PEM string) error {
	var privKeyEd25519 *string
	if privateKeyEd25519PEM != "" {
		privKeyEd25519 = &privateKeyEd25519PEM
	}
	err := s.queries().InsertActorCredentials(ctx, db.InsertActorCredentialsParams{
		ActorIri:             actorIRI,
		TenantID:             tenantID,
		Username:             username,
		PrivateKeyRsaPem:     privateKeyRSAPEM,
		PrivateKeyEd25519Pem: privKeyEd25519,
	})
	if err != nil {
		return fmt.Errorf("failed to write dual-key actor credentials to storage: %w", err)
	}
	return nil
}

func (s *PostgresStorage) IsDomainBlocked(ctx context.Context, domainName string) (bool, error) {
	return s.queries().IsDomainBlocked(ctx, domainName)
}

func (s *PostgresStorage) EnqueueInbound(ctx context.Context, id string, activityIRI, objectIRI string, tenantID int32, payload []byte) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer s.safeRollback(ctx, tx)
	queries := db.New(tx)

	queueID, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("parse inbound queue ID: %w", err)
	}

	if err := queries.EnqueueInboundActivity(ctx, db.EnqueueInboundActivityParams{
		ID:          queueID,
		ActivityIri: activityIRI,
		ObjectIri:   objectIRI,
		Payload:     payload,
	}); err != nil {
		return err
	}

	// Use tenantID directly to link delivery records seamlessly
	if err := queries.RecordTenantDelivery(ctx, db.RecordTenantDeliveryParams{
		ActivityIri: activityIRI,
		TenantID:    tenantID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *PostgresStorage) RecordActorInboxDelivery(ctx context.Context, actorIRI, activityIRI string) error {
	return s.queries().RecordActorInboxDelivery(ctx, db.RecordActorInboxDeliveryParams{ActorIri: actorIRI, ActivityIri: activityIRI})
}

func extractDomain(iri string) string {
	parts := strings.Split(iri, "://")
	if len(parts) < 2 {
		return ""
	}
	subParts := strings.Split(parts[1], "/")
	if len(subParts) == 0 {
		return ""
	}
	return strings.ToLower(subParts[0])
}

func (s *PostgresStorage) GetCollectionPayloads(ctx context.Context, actorIRI, collection string, limit, offset int) ([][]byte, error) {
	queries := s.queries()
	switch collection {
	case "inbox":
		return queries.GetInboxPayloads(ctx, db.GetInboxPayloadsParams{ActorIri: actorIRI, Limit: int32(limit), Offset: int32(offset)})
	case "outbox":
		return queries.GetOutboxPayloads(ctx, db.GetOutboxPayloadsParams{ActorIri: actorIRI, Limit: int32(limit), Offset: int32(offset)})
	case "pending_follows":
		return s.getPendingFollowsPayloads(ctx, actorIRI, limit, offset)
	default:
		return nil, fmt.Errorf("unsupported collection %q", collection)
	}
}

func (s *PostgresStorage) getPendingFollowsPayloads(ctx context.Context, actorIRI string, limit, offset int) ([][]byte, error) {
	tenantID, _ := ctx.Value(model.TenantIDKey).(int32)
	if tenantID == 0 {
		host := extractDomain(actorIRI)
		if host != "" {
			if tRow, err := s.queries().GetTenantByDomain(ctx, host); err == nil {
				tenantID = tRow.ID
			}
		}
	}
	if tenantID == 0 {
		tenantID = 1
	}

	query := `
		SELECT DISTINCT g.payload
		FROM rdf_graphs g
		JOIN rdf_statements rs_obj ON g.activity_id = rs_obj.subject
		JOIN rdf_statements rs_type ON g.activity_id = rs_type.subject
		WHERE rs_obj.tenant_id = $1
		  AND rs_obj.object = $2
		  AND (rs_obj.predicate ILIKE '%object%' OR rs_obj.predicate ILIKE '%as:object%')
		  AND rs_type.tenant_id = $1
		  AND rs_type.predicate ILIKE '%type%'
		  AND rs_type.object ILIKE '%Follow%'
		  AND NOT EXISTS (
			  SELECT 1 FROM rdf_statements rs_state
			  WHERE rs_state.subject = g.activity_id
				AND rs_state.tenant_id = $1
				AND (rs_state.predicate ILIKE '%accepted%' OR rs_state.predicate ILIKE '%rejected%' OR rs_state.predicate ILIKE '%result%')
		  )
		ORDER BY g.created_at DESC
		LIMIT $3 OFFSET $4;
	`
	rows, err := s.db.Query(ctx, query, tenantID, actorIRI, int32(limit), int32(offset))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payloads [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
	}
	return payloads, nil
}

func (s *PostgresStorage) ClaimInboundBatch(ctx context.Context, batchSize int) ([]model.InboundTask, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer s.safeRollback(ctx, tx)
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
	var errMsg *string
	if reason != "" {
		errMsg = &reason
	}
	return s.queries().MarkInboundFailed(ctx, db.MarkInboundFailedParams{ID: queueID, ErrorMessage: errMsg})
}

func (s *PostgresStorage) ClaimOutboundBatch(ctx context.Context, batchSize int) ([]model.OutboundTask, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer s.safeRollback(ctx, tx)
	queries := db.New(tx)
	rows, err := queries.ClaimOutboundTasks(ctx, int32(batchSize))
	if err != nil {
		return nil, err
	}
	ids := make([]pgtype.UUID, 0, len(rows))
	tasks := make([]model.OutboundTask, 0, len(rows))
	for _, row := range rows {
		id, err := uuidFromPG(row.ID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, row.ID)
		var attempts int
		if row.Attempts != nil {
			attempts = int(*row.Attempts)
		}
		tasks = append(tasks, model.OutboundTask{
			ID:          id.String(),
			ActivityIRI: row.ActivityIri,
			ActorIRI:    row.ActorIri,
			Payload:     row.Payload,
			Attempts:    attempts,
		})
	}
	if len(ids) > 0 {
		if err := queries.MarkOutboundProcessing(ctx, ids); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *PostgresStorage) MarkOutboundComplete(ctx context.Context, id string) error {
	queueID, err := parseUUID(id)
	if err != nil {
		return err
	}
	return s.queries().MarkOutboundComplete(ctx, queueID)
}

func (s *PostgresStorage) MarkOutboundFailed(ctx context.Context, id string, reason string) error {
	queueID, err := parseUUID(id)
	if err != nil {
		return err
	}

	// Fetch current attempts to calculate exponential backoff delay
	var attempts int32
	err = s.db.QueryRow(ctx, "SELECT attempts FROM outbound_activity_queue WHERE id = $1", queueID).Scan(&attempts)
	if err != nil {
		return err
	}

	// Calculate truncated exponential backoff delay: 2^attempts * BaseDelay
	baseDelay := 1 * time.Second
	maxDelay := 2 * time.Hour
	tempDelay := baseDelay * (1 << uint(attempts))
	if tempDelay > maxDelay || tempDelay < baseDelay {
		tempDelay = maxDelay
	}

	nextRunAt := time.Now().Add(tempDelay)

	var errMsg *string
	if reason != "" {
		errMsg = &reason
	}

	return s.queries().MarkOutboundFailed(ctx, db.MarkOutboundFailedParams{
		ID:           queueID,
		ErrorMessage: errMsg,
		NextRunAt:    pgtype.Timestamptz{Time: nextRunAt, Valid: true},
	})
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
	return s.queries().RegisterIdentityClone(ctx, db.RegisterIdentityCloneParams{IdentityGuid: guid, HubUrl: hubURL, IsLocal: &isLocal})
}

func (s *PostgresStorage) GetActorDualKeys(ctx context.Context, actorIRI string) (*model.ActorDualKeys, error) {
	row, err := s.queries().GetActorDualKeys(ctx, actorIRI)
	if err != nil {
		return nil, fmt.Errorf("failed fetching dual-key credential rows: %w", err)
	}

	var privKeyEd25519 string
	if row.PrivateKeyEd25519Pem != nil {
		privKeyEd25519 = *row.PrivateKeyEd25519Pem
	}
	return &model.ActorDualKeys{
		PrivateKeyRSAPEM:     row.PrivateKeyRsaPem,
		PrivateKeyEd25519PEM: privKeyEd25519,
	}, nil
}

func (s *PostgresStorage) CreateGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte) (int64, error) {
	return s.queries().CreateGraphVersion(ctx, db.CreateGraphVersionParams{ActivityID: activityIRI, ObjectIri: objectIRI, Payload: payload})
}

func (s *PostgresStorage) SaveGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer s.safeRollback(ctx, tx)
	queries := db.New(tx)
	graphID, err := queries.CreateGraphVersion(ctx, db.CreateGraphVersionParams{ActivityID: activityIRI, ObjectIri: objectIRI, Payload: payload})
	if err != nil {
		return err
	}

	// Convert human-readable string quads into lightweight integer QuadIDs
	quadIDs, err := s.toQuadIDs(ctx, queries, graphID, quads)
	if err != nil {
		return err
	}

	if err := s.saveQuadIDs(ctx, queries, quadIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStorage) SaveGraphVersionWithMedia(ctx context.Context, params port.MediaAttachmentParams) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer s.safeRollback(ctx, tx)
	queries := db.New(tx)

	// 1. Utilize GetTenantByDomain to retrieve the integer tenant ID.
	tenantRow, err := queries.GetTenantByDomain(ctx, params.TenantID)
	if err != nil {
		return fmt.Errorf("failed to resolve tenant %q for media ownership: %w", params.TenantID, err)
	}

	// 2. Pre-flight transactional quota guard
	// Aggregate active space footprint allocations within the current transaction scope.
	quota, err := queries.GetTenantStorageUsageAndCeiling(ctx, tenantRow.ID)
	if err != nil {
		return fmt.Errorf("pre-flight quota check failure: %w", err)
	}

	// Enforce hard ceiling threshold parameters before allocating database IDs or ledger metrics.
	if quota.CurrentUsageBytes+params.FileSize > quota.StorageCeilingBytes {
		return fmt.Errorf("payload rejected: storage ceiling threshold exceeded for tenant ID %d", tenantRow.ID)
	}

	// 3. Register the physical media file details globally inside the centralized registry bucket
	mediaID, err := queries.InsertMediaAttachment(ctx, db.InsertMediaAttachmentParams{
		ObjectName:   params.ObjectName,
		OriginalName: params.OriginalName,
		Sha256Hex:    params.SHA256Hex,
		ContentType:  params.ContentType,
		FileSize:     params.FileSize,
	})
	if err != nil {
		return fmt.Errorf("failed to register central media registry entry: %w", err)
	}

	// 4. Track local multi-tenant storage resource ownership metrics per actor/tenant boundary.
	err = queries.RegisterActorMediaOwnership(ctx, db.RegisterActorMediaOwnershipParams{
		ActorIri:          params.ActorIRI,
		TenantID:          tenantRow.ID,
		MediaAttachmentID: mediaID,
	})
	if err != nil {
		return fmt.Errorf("failed to commit tenant storage ownership mappings: %w", err)
	}

	// 5. Create the underlying core immutable activity graph version layer
	graphID, err := queries.CreateGraphVersion(ctx, db.CreateGraphVersionParams{
		ActivityID: params.ActivityIRI,
		ObjectIri:  params.ObjectIRI,
		Payload:    params.Payload,
	})
	if err != nil {
		return fmt.Errorf("failed to save graph version metadata: %w", err)
	}

	// 6. Connect media entries to graph tracking
	err = queries.LinkAttachmentToGraphVersion(ctx, db.LinkAttachmentToGraphVersionParams{
		GraphID:           graphID,
		MediaAttachmentID: mediaID,
	})
	if err != nil {
		return fmt.Errorf("failed to connect media entry to graph version: %w", err)
	}

	// 7. Expand string elements out to dictionary indices
	quadIDs, err := s.toQuadIDs(ctx, queries, graphID, params.Quads)
	if err != nil {
		return fmt.Errorf("failed to parse index strings to dictionary: %w", err)
	}

	// 8. Commit the indexing quad arrays
	if err := s.saveQuadIDs(ctx, queries, quadIDs); err != nil {
		return fmt.Errorf("failed to write indexing quad arrays: %w", err)
	}

	return tx.Commit(ctx)
}

// VerifyIncomingQuota evaluates the current space utilization against the multi-tenant block ceiling.
func (s *PostgresStorage) VerifyIncomingQuota(ctx context.Context, tenantID int32, incomingSizeBytes int64) (bool, error) {
	// Wrap the db instance pool with the type-safe compiled sqlc engine block
	queriesEngine := db.New(s.db)

	metrics, err := queriesEngine.GetTenantStorageUsageAndCeiling(ctx, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to fetch multi-tenant storage metrics: %w", err)
	}

	// Intercept hard ceilings before writing frames or chunks to any storage node
	projectedUsage := metrics.CurrentUsageBytes + incomingSizeBytes
	if projectedUsage > metrics.StorageCeilingBytes {
		return false, nil // Limit breached safely intercepted
	}

	return true, nil // Allocation approved
}

// RemoveMediaRecord clears temporary space weight rows if an upscale loop fails mid-verifications.
func (s *PostgresStorage) RemoveMediaRecord(ctx context.Context, objectName string) error {
	queriesEngine := db.New(s.db)

	if err := queriesEngine.RemoveMediaAttachment(ctx, objectName); err != nil {
		return fmt.Errorf("failed to execute compensating database row prune: %w", err)
	}
	return nil
}

func (s *PostgresStorage) SaveQuads(ctx context.Context, quads []model.Quad) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer s.safeRollback(ctx, tx)
	queries := db.New(tx)

	quadIDs, err := s.toQuadIDs(ctx, queries, 0, quads)
	if err != nil {
		return err
	}

	if err := s.saveQuadIDs(ctx, queries, quadIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// toQuadIDs converts a batch of string-based Quads into compact, integer-indexed QuadID structures.
func (s *PostgresStorage) toQuadIDs(ctx context.Context, queries *db.Queries, defaultGraphID int64, quads []model.Quad) ([]model.QuadID, error) {
	quadIDs := make([]model.QuadID, len(quads))
	for i, quad := range quads {
		graphID := quad.GraphID
		if defaultGraphID != 0 {
			graphID = defaultGraphID
		}

		subID, err := s.dictionaryID(ctx, queries, quad.Subject)
		if err != nil {
			return nil, err
		}
		predID, err := s.dictionaryID(ctx, queries, quad.Predicate)
		if err != nil {
			return nil, err
		}

		var objID int64
		var literalValue string
		if quad.IsLiteral() {
			literalValue = quad.Object
		} else {
			objID, err = s.dictionaryID(ctx, queries, quad.Object)
			if err != nil {
				return nil, err
			}
		}

		quadIDs[i] = model.QuadID{
			GraphID:      graphID,
			SubjectID:    subID,
			PredicateID:  predID,
			ObjectID:     objID,
			IsLiteral:    quad.IsLiteral(),
			LiteralValue: literalValue,
		}
	}
	return quadIDs, nil
}

var (
	trueVal  = true
	falseVal = false
)

func stringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func int64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func boolPtr(v bool) *bool {
	if v {
		return &trueVal
	}
	return &falseVal
}

// saveQuadIDs natively processes and writes clean slices of model.QuadID to the database.
func (s *PostgresStorage) saveQuadIDs(ctx context.Context, queries *db.Queries, quadIDs []model.QuadID) error {
	for _, qID := range quadIDs {
		var literalValue *string
		if qID.IsLiteral {
			literalValue = stringPtr(qID.LiteralValue)
		}
		params := db.InsertQuadParams{
			GraphID:      qID.GraphID,
			SubjectID:    qID.SubjectID,
			PredicateID:  qID.PredicateID,
			ObjectID:     int64Ptr(qID.ObjectID),
			IsLiteral:    boolPtr(qID.IsLiteral),
			LiteralValue: literalValue,
		}
		if err := queries.InsertQuad(ctx, params); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStorage) SaveQuadIDs(ctx context.Context, quadIDs []model.QuadID) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer s.safeRollback(ctx, tx)
	if err := s.saveQuadIDs(ctx, db.New(tx), quadIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStorage) getExistingDictionaryID(ctx context.Context, value string) (int64, error) {
	if id, found := s.cache.GetID(value); found {
		return id, nil
	}
	id, err := s.queries().GetDictionaryID(ctx, value)
	if err != nil {
		return 0, err
	}
	s.cache.Set(value, id)
	return id, nil
}

func (s *PostgresStorage) RemoveQuadEdge(ctx context.Context, subject, predicate, object string) error {
	subID, err := s.getExistingDictionaryID(ctx, subject)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	predID, err := s.getExistingDictionaryID(ctx, predicate)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	var objectID *int64
	var literalValue *string

	objID, err := s.getExistingDictionaryID(ctx, object)
	if err == nil {
		objectID = &objID
	} else if errors.Is(err, pgx.ErrNoRows) {
		literalValue = &object
	} else {
		return err
	}

	return s.queries().RemoveQuadEdge(ctx, db.RemoveQuadEdgeParams{
		SubjectID:    subID,
		PredicateID:  predID,
		ObjectID:     objectID,
		LiteralValue: literalValue,
	})
}

func (s *PostgresStorage) GetLatestPayload(ctx context.Context, objectIRI string) ([]byte, error) {
	payload, err := s.queries().GetLatestPayload(ctx, objectIRI)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return payload, err
}

func (s *PostgresStorage) GetLikesForObject(ctx context.Context, objectIRI string) ([]string, error) {
	return s.queries().GetEngagementActivities(ctx, db.GetEngagementActivitiesParams{
		Value:   objectIRI,
		Value_2: model.TypeLike,
	})
}

func (s *PostgresStorage) GetSharesForObject(ctx context.Context, objectIRI string) ([]string, error) {
	return s.queries().GetEngagementActivities(ctx, db.GetEngagementActivitiesParams{
		Value:   objectIRI,
		Value_2: model.TypeAnnounce,
	})
}

func (s *PostgresStorage) GetRepliesForObject(ctx context.Context, objectIRI string) ([]string, error) {
	return s.queries().GetRepliesByObject(ctx, objectIRI)
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
		if row.IsLiteral != nil && *row.IsLiteral {
			objType = model.Literal
		}
		quads = append(quads, model.Quad{
			GraphID:   row.GraphID,
			Subject:   subjectIRI,
			Predicate: row.Predicate,
			Object:    row.Object,
			ObjType:   objType,
		})
	}
	return quads, nil
}

func (s *PostgresStorage) GetStatementsBySubjectIsolated(ctx context.Context, subjectIRI string, tenantID int32) ([]model.Quad, error) {
	rows, err := s.queries().GetStatementsBySubjectIsolated(ctx, db.GetStatementsBySubjectIsolatedParams{
		Subject:  subjectIRI,
		TenantID: tenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []model.Quad{}, nil
		}
		return nil, err
	}
	quads := make([]model.Quad, 0, len(rows))
	for _, row := range rows {
		objType := model.NamedNode
		if row.IsLiteral != nil && *row.IsLiteral {
			objType = model.Literal
		}
		quads = append(quads, model.Quad{
			Subject:   subjectIRI,
			Predicate: row.Predicate,
			Object:    row.Object,
			ObjType:   objType,
		})
	}
	return quads, nil
}

func (s *PostgresStorage) GetTenantIDByActivityIRI(ctx context.Context, activityIRI string) (int32, error) {
	tenantID, err := s.queries().GetTenantIDByActivityIRI(ctx, activityIRI)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return tenantID, nil
}

func (s *PostgresStorage) dictionaryID(ctx context.Context, queries *db.Queries, value string) (int64, error) {
	if id, found := s.cache.GetID(value); found {
		return id, nil
	}

	// Leverage a pure database UPSERT operation or a safety query fallback routine inside
	// your schema wrapper to catch or handle unique constraint violations under concurrent execution windows.
	id, err := queries.InsertDictionaryValue(ctx, value)
	if err != nil {
		// Fallback lookup case to prevent unique tracking constraint abort exceptions
		var lookupErr error
		id, lookupErr = queries.GetDictionaryID(ctx, value)
		if lookupErr == nil {
			s.cache.Set(value, id)
			return id, nil
		}
		return 0, err
	}

	s.cache.Set(value, id)
	return id, nil
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

// Clean context lifecycle binding wrapper to eliminate hanging database network sockets.
func (s *PostgresStorage) safeRollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

func (s *PostgresStorage) GetActorIRIByUsername(ctx context.Context, tenantID int32, username string) (string, error) {
	row, err := s.queries().GetActorCredentialsByUsername(ctx, db.GetActorCredentialsByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
	if err != nil {
		return "", fmt.Errorf("failed fetching actor row for username lookup: %w", err)
	}
	return row.ActorIri, nil
}

func (s *PostgresStorage) GetActorIRIByAlias(ctx context.Context, alias string) (string, error) {
	subject, err := s.queries().GetActorIRIByAlias(ctx, alias)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("actor IRI not found for alias %q: %w", alias, err)
		}
		return "", fmt.Errorf("failed to lookup actor IRI by alias: %w", err)
	}
	return subject, nil
}

func (s *PostgresStorage) GetActorProfileFromGraph(ctx context.Context, tenantID int32, username string) (*model.ActorProfile, error) {
	tenantDomain, err := s.queries().GetTenantDomainByID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to map tenant context to domain name: %w", err)
	}

	// Dynamic path-agnostic prefix matching using the domain boundary (isolating by tenant)
	tenantActorPrefix := fmt.Sprintf("https://%s/%%", tenantDomain)

	rows, err := s.queries().GetActorQuadsByUsername(ctx, db.GetActorQuadsByUsernameParams{
		Username:     username,
		TenantPrefix: tenantActorPrefix,
	})
	if err != nil {
		return nil, fmt.Errorf("failed scanning quad store for actor handle: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("actor profile graph not found for handle %q", username)
	}

	profile := &model.ActorProfile{
		IRI:      rows[0].Subject,
		Username: username,
	}

	// 1. Process fields iteratively for the UsernameRow slice layout
	for _, row := range rows {
		s.mapQuadPredicateToProfile(profile, row.Predicate, row.Object)
	}

	return s.finalizeProfileIdentity(profile), nil
}

func (s *PostgresStorage) GetActorProfileByIRI(ctx context.Context, tenantID int32, iri string) (*model.ActorProfile, error) {
	rows, err := s.queries().GetActorQuadsByIRI(ctx, iri)
	if err != nil {
		return nil, fmt.Errorf("failed scanning quad store for actor IRI: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("actor profile graph not found for IRI %q", iri)
	}

	profile := &model.ActorProfile{
		IRI: iri,
	}

	// 2. Process fields iteratively for the IRIRow slice layout (Resolving type lock)
	for _, row := range rows {
		s.mapQuadPredicateToProfile(profile, row.Predicate, row.Object)
	}

	return s.finalizeProfileIdentity(profile), nil
}

// Cryptographic Public Key History Persistence Handlers

func (s *PostgresStorage) ArchiveKeyHistory(ctx context.Context, actorIRI string, keyType string, publicKeyPEM string, validFrom time.Time, validTo time.Time) error {
	// CORRECTED: Wrap standard Go time.Time structs into type-safe pgtype.Timestamptz wrappers
	err := s.queries().InsertHistoricalKey(ctx, db.InsertHistoricalKeyParams{
		ActorIri:     actorIRI,
		KeyType:      keyType,
		PublicKeyPem: publicKeyPEM,
		ValidFrom:    pgtype.Timestamptz{Time: validFrom, Valid: true},
		ValidTo:      pgtype.Timestamptz{Time: validTo, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to archive historical %s public key entry for %s: %w", keyType, actorIRI, err)
	}
	return nil
}

func (s *PostgresStorage) GetHistoricalKey(ctx context.Context, actorIRI string, keyType string, signedAt time.Time) (string, error) {
	// Execute lookup passing our timestamp directly into the single named field
	publicKeyPEM, err := s.queries().FindHistoricalKeyInWindow(ctx, db.FindHistoricalKeyInWindowParams{
		ActorIri: actorIRI,
		KeyType:  keyType,
		SignedAt: pgtype.Timestamptz{Time: signedAt, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("no historical %s key found for actor %s matching timestamp %s", keyType, actorIRI, signedAt.Format(time.RFC3339))
		}
		return "", fmt.Errorf("failed querying historical key verification logs: %w", err)
	}
	return publicKeyPEM, nil
}

// mapQuadPredicateToProfile unifies graph edge vocabulary mappings cleanly
func (s *PostgresStorage) mapQuadPredicateToProfile(profile *model.ActorProfile, predicate, object string) {
	cleanObject := strings.Trim(object, `"'`)

	switch predicate {
	case model.PredicatePreferredUsername:
		profile.Username = cleanObject
	case model.PredicatePublicKeyPem:
		profile.PublicKeyPEM = cleanObject
	case model.PredicateNomadGUID:
		profile.NomadGUID = cleanObject
	}
}

// finalizeProfileIdentity extracts stable protocol UUIDv4 blocks inside a localized routine (Blueprint Section 3.1)
func (s *PostgresStorage) finalizeProfileIdentity(profile *model.ActorProfile) *model.ActorProfile {
	if parts := strings.Split(profile.IRI, "/"); len(parts) > 0 {
		profile.UUID = parts[len(parts)-1]
	}
	return profile
}
