package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/port/portmock"
)

func TestGenericHandler_GetProfile_Success(t *testing.T) {
	mc := minimock.NewController(t)

	storage := portmock.NewStoragePortMock(mc)
	storage.GetLatestPayloadMock.Inspect(func(ctx context.Context, objectIRI string) {
		if objectIRI != "https://local.example/actor/alice" {
			t.Errorf("expected IRI 'https://local.example/actor/alice', got %s", objectIRI)
		}
	}).Return([]byte(`{"id":"https://local.example/actor/alice","type":"Person","preferredUsername":"alice"}`), nil)

	handler := inhttp.NewGenericHandler(storage)

	req := httptest.NewRequest(http.MethodGet, "/actor/alice", nil)
	req.Host = "local.example"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/activity+json") {
		t.Errorf("expected Content-Type to contain application/activity+json, got %s", contentType)
	}
}

func TestGenericHandler_PostSharedInbox_Success(t *testing.T) {
	mc := minimock.NewController(t)

	storage := portmock.NewStoragePortMock(mc)
	storage.EnqueueInboundMock.Inspect(func(ctx context.Context, id, activityIRI, objectIRI, targetDomain string, payload []byte) {
		if activityIRI != "https://remote.com/activity-1" {
			t.Errorf("expected activityIRI to be 'https://remote.com/activity-1', got %s", activityIRI)
		}
		if targetDomain != "local.example" {
			t.Errorf("expected targetDomain to be 'local.example', got %s", targetDomain)
		}
	}).Return(nil)

	handler := inhttp.NewGenericHandler(storage)

	payload := []byte(`{"id":"https://remote.com/activity-1","type":"Create","object":{"id":"https://remote.com/object-1"}}`)
	req := httptest.NewRequest(http.MethodPost, "/inbox", bytes.NewReader(payload))
	req.Host = "local.example"
	rec := httptest.NewRecorder()

	ctx := context.WithValue(req.Context(), middleware.AuthenticatedActorKey, "https://remote.com/actor")
	req = req.WithContext(ctx)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202 Accepted, got %d", rec.Code)
	}
}

func TestGenericHandler_PostDirectInbox_Success(t *testing.T) {
	mc := minimock.NewController(t)

	storage := portmock.NewStoragePortMock(mc)
	storage.EnqueueInboundMock.Inspect(func(ctx context.Context, id, activityIRI, objectIRI, targetDomain string, payload []byte) {
		if activityIRI != "https://remote.com/activity-1" {
			t.Errorf("expected activityIRI to be 'https://remote.com/activity-1', got %s", activityIRI)
		}
		if objectIRI != "https://remote.com/object-1" {
			t.Errorf("expected objectIRI to be 'https://remote.com/object-1', got %s", objectIRI)
		}
		if targetDomain != "local.example" {
			t.Errorf("expected targetDomain to be 'local.example', got %s", targetDomain)
		}
	}).Return(nil)

	handler := inhttp.NewGenericHandler(storage)

	payload := []byte(`{"id":"https://remote.com/activity-1","type":"Create","object":{"id":"https://remote.com/object-1"}}`)
	req := httptest.NewRequest(http.MethodPost, "/actor/alice/inbox", bytes.NewReader(payload))
	req.Host = "local.example"
	rec := httptest.NewRecorder()

	ctx := context.WithValue(req.Context(), middleware.AuthenticatedActorKey, "https://remote.com/actor")
	req = req.WithContext(ctx)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202 Accepted, got %d", rec.Code)
	}
}
