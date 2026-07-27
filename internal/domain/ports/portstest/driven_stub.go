// File: /internal/domain/ports/portstest/driven_stub.go
package portstest

import (
	"context"
	"io"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports"
)

// UnimplementedStoragePort implements every method of ports.StoragePort with non-operational, zero-value fallbacks.
// Embed this into any test mock struct across your application to inherit default behaviors and maintain
// interface compliance automatically when new methods are added to ports.StoragePort.
type UnimplementedStoragePort struct{}

// Assert at compilation that this stub fully satisfies the ports.StoragePort interface contract
var _ ports.StoragePort = (*UnimplementedStoragePort)(nil)

// Domain Routing & Multi-Tenant Isolation Stubs

func (UnimplementedStoragePort) GetOrCreateTenantByDomain(ctx context.Context, domainName string) (int32, error) {
	return 0, nil
}

func (UnimplementedStoragePort) HasActorCredential(ctx context.Context, tenantID int32, username string) (bool, error) {
	return false, nil
}

func (UnimplementedStoragePort) CreateActorCredential(ctx context.Context, actorIRI string, tenantID int32, username string, privateKeyPEM string) error {
	return nil
}

func (UnimplementedStoragePort) GetTenantStorageMetrics(ctx context.Context, tenantID int32) (int64, int64, error) {
	return 0, 0, nil
}

func (UnimplementedStoragePort) IsDomainBlocked(ctx context.Context, domainName string) (bool, error) {
	return false, nil
}

func (UnimplementedStoragePort) EnqueueInbound(ctx context.Context, id string, activityIRI, objectIRI, targetDomain string, payload []byte) error {
	return nil
}

func (UnimplementedStoragePort) ClaimInboundBatch(ctx context.Context, batchSize int) ([]model.InboundTask, error) {
	return nil, nil
}

func (UnimplementedStoragePort) MarkInboundComplete(ctx context.Context, id string) error {
	return nil
}

func (UnimplementedStoragePort) MarkInboundFailed(ctx context.Context, id string, reason string) error {
	return nil
}

func (UnimplementedStoragePort) RecordActorInboxDelivery(ctx context.Context, actorIRI, activityIRI string) error {
	return nil
}

// Nomadic Identity Management Stubs

func (UnimplementedStoragePort) GetNomadicIdentity(ctx context.Context, guid string) (*model.NomadicIdentity, error) {
	return nil, nil
}

func (UnimplementedStoragePort) UpsertNomadicIdentity(ctx context.Context, identity *model.NomadicIdentity) error {
	return nil
}

func (UnimplementedStoragePort) RegisterIdentityClone(ctx context.Context, guid string, hubURL string, isLocal bool) error {
	return nil
}

func (UnimplementedStoragePort) GetActorPrivateKey(ctx context.Context, actorIRI string) (string, error) {
	return "", nil
}

// Core RDF Event Sourcing Write Operations Stubs

func (UnimplementedStoragePort) CreateGraphVersion(ctx context.Context, activityIRI, objectIRI string, payload []byte) (int64, error) {
	return 0, nil
}

func (UnimplementedStoragePort) SaveQuads(ctx context.Context, quads []model.Quad) error {
	return nil
}

func (UnimplementedStoragePort) SaveQuadIDs(ctx context.Context, quadIDs []model.QuadID) error {
	return nil
}

func (UnimplementedStoragePort) RemoveQuadEdge(ctx context.Context, subject, predicate, object string) error {
	return nil
}

// Core RDF Graph Read Operations Stubs

func (UnimplementedStoragePort) GetLatestPayload(ctx context.Context, objectIRI string) ([]byte, error) {
	return nil, nil
}

func (UnimplementedStoragePort) StreamQuadsBySubject(ctx context.Context, subjectIRI string) ([]model.Quad, error) {
	return nil, nil
}

func (UnimplementedStoragePort) GetCollectionPayloads(ctx context.Context, actorIRI, collection string, limit, offset int) ([][]byte, error) {
	return nil, nil
}

func (UnimplementedStoragePort) GetActorProfileFromGraph(ctx context.Context, tenantID int32, username string) (*model.ActorProfile, error) {
	return nil, nil
}

func (UnimplementedStoragePort) GetActorProfileByIRI(ctx context.Context, tenantID int32, iri string) (*model.ActorProfile, error) {
	return nil, nil
}

// UnimplementedMediaStoragePort provides zero-value stubs for ports.MediaStoragePort.
type UnimplementedMediaStoragePort struct{}

var _ ports.MediaStoragePort = (*UnimplementedMediaStoragePort)(nil)

func (UnimplementedMediaStoragePort) PutObject(ctx context.Context, objectName string, reader io.Reader, contentType string) (string, string, error) {
	return "", "", nil
}

func (UnimplementedMediaStoragePort) DeleteObject(ctx context.Context, objectName string) error {
	return nil
}

// UnimplementedJSONLDParserPort provides zero-value stubs for ports.JSONLDParserPort.
type UnimplementedJSONLDParserPort struct{}

var _ ports.JSONLDParserPort = (*UnimplementedJSONLDParserPort)(nil)

func (UnimplementedJSONLDParserPort) ToQuads(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error) {
	return nil, nil
}
