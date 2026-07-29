package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gojuno/minimock/v3"
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/domain/service"
)

func TestProcessInboundTask_IsolatedDeletes(t *testing.T) {
	mc := minimock.NewController(t)
	defer mc.Finish()

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)

	svc := service.NewActivityService(mockStorage, mockParser, mockMedia)

	// Context with Tenant ID 1
	ctx := context.WithValue(context.Background(), model.TenantIDKey, int32(1))

	// Payload of incoming Delete activity
	payload := []byte(`{
		"id": "https://tenant-a.com/activity/delete-1",
		"type": "Delete",
		"actor": "https://tenant-a.com/actor/alice",
		"object": "https://tenant-a.com/note/123"
	}`)

	task := model.InboundTask{
		ID:          "task-1",
		ActivityIRI: "https://tenant-a.com/activity/delete-1",
		ObjectIRI:   "https://tenant-a.com/note/123",
		Payload:     payload,
	}

	t.Run("Graceful early exit (nil code exit) when target is isolated or missing", func(t *testing.T) {
		// Mock Actor quads containing their active public key graph entry
		mockStorage.StreamQuadsBySubjectMock.Expect(ctx, "https://tenant-a.com/actor/alice").Return([]model.Quad{
			{Subject: "https://tenant-a.com/actor/alice", Predicate: model.PredicatePublicKeyPem, Object: "RSA-KEY-PEM"},
		}, nil)

		// Mock isolated subject query returns 0 rows (missing or isolated target)
		mockStorage.GetStatementsBySubjectIsolatedMock.Expect(ctx, "https://tenant-a.com/note/123", int32(1)).Return([]model.Quad{}, nil)

		err := svc.ProcessInboundTask(ctx, task)
		if err != nil {
			t.Fatalf("expected nil error (graceful drop), got: %v", err)
		}
	})

	t.Run("Security violation when target exists but owned by different actor", func(t *testing.T) {
		// Mock Actor quads containing their active public key graph entry
		mockStorage.StreamQuadsBySubjectMock.Expect(ctx, "https://tenant-a.com/actor/alice").Return([]model.Quad{
			{Subject: "https://tenant-a.com/actor/alice", Predicate: model.PredicatePublicKeyPem, Object: "RSA-KEY-PEM"},
		}, nil)

		// Mock isolated subject query returns target quads owned by Bob
		mockStorage.GetStatementsBySubjectIsolatedMock.Expect(ctx, "https://tenant-a.com/note/123", int32(1)).Return([]model.Quad{
			{Subject: "https://tenant-a.com/note/123", Predicate: "https://www.w3.org/ns/activitystreams#attributedTo", Object: "https://tenant-a.com/actor/bob"},
		}, nil)

		err := svc.ProcessInboundTask(ctx, task)
		if err == nil {
			t.Fatal("expected error due to security violation, got nil")
		}
	})

	t.Run("Successful processing when target exists and owned by the same actor", func(t *testing.T) {
		// Mock Actor quads containing their active public key graph entry
		mockStorage.StreamQuadsBySubjectMock.Expect(ctx, "https://tenant-a.com/actor/alice").Return([]model.Quad{
			{Subject: "https://tenant-a.com/actor/alice", Predicate: model.PredicatePublicKeyPem, Object: "RSA-KEY-PEM"},
		}, nil)

		// Mock isolated subject query returns target quads owned by Alice
		mockStorage.GetStatementsBySubjectIsolatedMock.Expect(ctx, "https://tenant-a.com/note/123", int32(1)).Return([]model.Quad{
			{Subject: "https://tenant-a.com/note/123", Predicate: "https://www.w3.org/ns/activitystreams#attributedTo", Object: "https://tenant-a.com/actor/alice"},
		}, nil)

		// Save/parse quads expectations
		parsedQuads := []model.Quad{
			{Subject: "https://tenant-a.com/activity/delete-1", Predicate: "type", Object: "Delete"},
		}
		mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error) {
			return parsedQuads, nil
		})
		mockStorage.SaveGraphVersionMock.Set(func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
			return nil
		})

		// GetActorDualKeys expectations for expanding followers & local deliveries
		mockStorage.GetActorDualKeysMock.Return(nil, errors.New("not local")) // Skip target deliveries

		err := svc.ProcessInboundTask(ctx, task)
		if err != nil {
			t.Fatalf("expected successful delete processing, got error: %v", err)
		}
	})
}
