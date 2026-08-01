package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"

	"github.com/gojuno/minimock/v3"
)

func TestActivityService_ValidateInboundActivity_SelfQuote(t *testing.T) {
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockFetcher := portmock.NewRemoteFetcherMock(mc)

	svc := NewActivityService(mockStorage, mockParser, mockMedia, mockFetcher, ActivityServiceConfig{})
	ctx := context.Background()

	// Alice quotes her own post:
	actor := "https://sprezz.net/actor/alice"
	quotedIRI := "https://sprezz.net/statuses/101"

	// Mocking getQuotedPostAuthor: Alice's post is local, query storage.
	mockStorage.StreamQuadsBySubjectMock.When(ctx, quotedIRI).Then([]model.Quad{
		{Subject: quotedIRI, Predicate: model.PredicateAttributedTo, Object: actor},
	}, nil)

	payload := []byte(`{
		"type": "Create",
		"actor": "` + actor + `",
		"object": {
			"type": "Note",
			"id": "https://sprezz.net/statuses/102",
			"attributedTo": "` + actor + `",
			"quote": "` + quotedIRI + `"
		}
	}`)

	err := svc.validateInboundActivity(ctx, "https://sprezz.net/activity/1", payload)
	if err != nil {
		t.Fatalf("Expected self-quote validation to succeed, got error: %v", err)
	}
}

func TestActivityService_ValidateInboundActivity_ThirdPartyQuote_NoStamp_Fails(t *testing.T) {
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockFetcher := portmock.NewRemoteFetcherMock(mc)

	svc := NewActivityService(mockStorage, mockParser, mockMedia, mockFetcher, ActivityServiceConfig{})
	ctx := context.Background()

	// Bob quotes Alice's post:
	actor := "https://remote.com/actor/bob"
	quotedIRI := "https://sprezz.net/statuses/101"

	// Alice's post is local, query storage.
	mockStorage.StreamQuadsBySubjectMock.When(ctx, quotedIRI).Then([]model.Quad{
		{Subject: quotedIRI, Predicate: model.PredicateAttributedTo, Object: "https://sprezz.net/actor/alice"},
	}, nil)

	payload := []byte(`{
		"type": "Create",
		"actor": "` + actor + `",
		"object": {
			"type": "Note",
			"id": "https://remote.com/statuses/201",
			"attributedTo": "` + actor + `",
			"quote": "` + quotedIRI + `"
		}
	}`)

	err := svc.validateInboundActivity(ctx, "https://remote.com/activity/2", payload)
	if err == nil {
		t.Fatal("Expected process inbound task to fail!")
	}

	if !strings.Contains(err.Error(), "third-party quote post") || !strings.Contains(err.Error(), "lacks a valid FEP-044f quote authorization stamp") {
		t.Fatalf("Expected validation error about missing quote authorization, got: %v", err)
	}
}

func TestActivityService_ValidateInboundActivity_ThirdPartyQuote_WithStamp_Success(t *testing.T) {
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockFetcher := portmock.NewRemoteFetcherMock(mc)

	svc := NewActivityService(mockStorage, mockParser, mockMedia, mockFetcher, ActivityServiceConfig{})
	ctx := context.Background()

	actor := "https://remote.com/actor/bob"
	quotedIRI := "https://sprezz.net/statuses/101"
	quotedAuthor := "https://sprezz.net/actor/alice"
	stampIRI := "https://sprezz.net/actor/alice/stamps/1"
	quotePostID := "https://remote.com/statuses/201"

	// Alice's post is local
	mockStorage.StreamQuadsBySubjectMock.When(ctx, quotedIRI).Then([]model.Quad{
		{Subject: quotedIRI, Predicate: model.PredicateAttributedTo, Object: quotedAuthor},
	}, nil)

	// Fetch stamp over the wire (mock remote fetch)
	stampPayload := map[string]interface{}{
		"type":              "QuoteAuthorization",
		"id":                stampIRI,
		"attributedTo":      quotedAuthor,
		"interactingObject": quotePostID,
		"interactionTarget": quotedIRI,
	}
	stampBytes, _ := json.Marshal(stampPayload)

	mockStorage.StreamQuadsBySubjectMock.When(ctx, stampIRI).Then(nil, nil) // Simulate local cache miss for stamp
	mockFetcher.FetchSignedMock.When(ctx, stampIRI, "", "", "").Then(stampBytes, nil)

	payload := []byte(`{
		"type": "Create",
		"actor": "` + actor + `",
		"object": {
			"type": "Note",
			"id": "` + quotePostID + `",
			"attributedTo": "` + actor + `",
			"quote": "` + quotedIRI + `",
			"quoteAuthorization": "` + stampIRI + `"
		}
	}`)

	err := svc.validateInboundActivity(ctx, "https://remote.com/activity/2", payload)
	if err != nil {
		t.Fatalf("Expected validation to succeed, got error: %v", err)
	}
}

func TestActivityService_HandleQuoteRequest_AutoApproval_Public(t *testing.T) {
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockFetcher := portmock.NewRemoteFetcherMock(mc)
	mockForwarder := portmock.NewOutboundDispatcherMock(mc)

	svc := NewActivityService(mockStorage, mockParser, mockMedia, mockFetcher, ActivityServiceConfig{}, mockForwarder)
	ctx := context.Background()

	localActorIRI := "https://sprezz.net/actor/alice"
	quotedIRI := "https://sprezz.net/statuses/101"
	senderIRI := "https://remote.com/actor/bob"
	quotePostID := "https://remote.com/statuses/201"

	// Mock quoted post in storage (it is public)
	mockStorage.StreamQuadsBySubjectMock.When(ctx, quotedIRI).Then([]model.Quad{
		{Subject: quotedIRI, Predicate: model.PredicateAttributedTo, Object: localActorIRI},
		{Subject: quotedIRI, Predicate: model.PredicateTo, Object: model.PublicAudience},
	}, nil)
	mockStorage.StreamQuadsBySubjectMock.When(ctx, localActorIRI).Then(nil, nil) // Mock get followers

	// Mock actor dual keys for local actor
	mockStorage.GetActorDualKeysMock.When(ctx, localActorIRI).Then(&model.ActorDualKeys{
		PrivateKeyRSAPEM:     "rsa",
		PrivateKeyEd25519PEM: "ed",
	}, nil)
	mockStorage.GetActorDualKeysMock.When(ctx, "https://remote.com/statuses/201/quote-request").Then(nil, errors.New("not local")) // Mock check for group
	mockStorage.GetActorDualKeysMock.When(ctx, "https://remote.com/actor/bob").Then(nil, errors.New("not local"))                  // Mock check for delivery
	mockStorage.StreamQuadsBySubjectMock.When(ctx, "https://remote.com").Then([]model.Quad{
		{Subject: "https://remote.com", Predicate: model.PredicateInbox, Object: "https://remote.com/inbox"},
	}, nil)

	// ToQuads for stamp creation and saving
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, subjectIRI string, payload []byte) (quads []model.Quad, err error) {
		return []model.Quad{{Subject: subjectIRI}}, nil
	})
	mockStorage.SaveGraphVersionMock.Set(func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
		return nil
	})

	// Dispatch outbound mock
	mockForwarder.ForwardFederatedActivityMock.Set(func(ctx context.Context, targetInbox, keyID, rsaPEM, edPEM string, payload []byte) error {
		var act map[string]interface{}
		_ = json.Unmarshal(payload, &act)
		if act["type"] != "Accept" {
			t.Errorf("Expected dispatched activity type to be Accept, got %v", act["type"])
		}
		if act["actor"] != localActorIRI {
			t.Errorf("Expected actor to be local actor %s, got %v", localActorIRI, act["actor"])
		}
		return nil
	})

	payload := []byte(`{
		"type": "QuoteRequest",
		"id": "https://remote.com/statuses/201/quote-request",
		"actor": "` + senderIRI + `",
		"object": "` + quotedIRI + `",
		"instrument": {
			"type": "Note",
			"id": "` + quotePostID + `",
			"attributedTo": "` + senderIRI + `",
			"quote": "` + quotedIRI + `"
		}
	}`)

	task := model.InboundTask{
		ActivityIRI: "https://remote.com/statuses/201/quote-request",
		ObjectIRI:   "https://remote.com/statuses/201/quote-request",
		Payload:     payload,
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected auto-approval handleQuoteRequest to succeed, got error: %v", err)
	}
}

func TestActivityService_HandleQuoteRequest_ManualPolicy_Pending(t *testing.T) {
	mc := minimock.NewController(t)

	mockStorage := portmock.NewStorageAndGraphWriterMock(mc)
	mockParser := portmock.NewJSONLDParserPortMock(mc)
	mockMedia := portmock.NewMediaStoragePortMock(mc)
	mockFetcher := portmock.NewRemoteFetcherMock(mc)
	mockForwarder := portmock.NewOutboundDispatcherMock(mc)

	svc := NewActivityService(mockStorage, mockParser, mockMedia, mockFetcher, ActivityServiceConfig{}, mockForwarder)
	ctx := context.Background()

	localActorIRI := "https://sprezz.net/actor/alice"
	quotedIRI := "https://sprezz.net/statuses/101"
	senderIRI := "https://remote.com/actor/bob"
	quotePostID := "https://remote.com/statuses/201"

	// Mock quoted post with explicit manual quote policy
	mockStorage.StreamQuadsBySubjectMock.When(ctx, quotedIRI).Then([]model.Quad{
		{Subject: quotedIRI, Predicate: model.PredicateAttributedTo, Object: localActorIRI},
		{Subject: quotedIRI, Predicate: "https://gotosocial.org/ns#interactionPolicy", Object: "manual"},
	}, nil)

	mockStorage.GetActorDualKeysMock.When(ctx, localActorIRI).Then(&model.ActorDualKeys{}, nil)
	mockStorage.GetActorDualKeysMock.When(ctx, "https://remote.com/statuses/201/quote-request").Then(nil, errors.New("not local")) // Mock check for group

	// Mock parser and save
	mockParser.ToQuadsMock.Set(func(ctx context.Context, graphID int64, subjectIRI string, payload []byte) (quads []model.Quad, err error) {
		return []model.Quad{{Subject: subjectIRI}}, nil
	})
	mockStorage.SaveGraphVersionMock.Set(func(ctx context.Context, activityIRI, objectIRI string, payload []byte, quads []model.Quad) error {
		return nil
	})

	payload := []byte(`{
		"type": "QuoteRequest",
		"id": "https://remote.com/statuses/201/quote-request",
		"actor": "` + senderIRI + `",
		"object": "` + quotedIRI + `",
		"instrument": {
			"type": "Note",
			"id": "` + quotePostID + `",
			"attributedTo": "` + senderIRI + `",
			"quote": "` + quotedIRI + `"
		}
	}`)

	task := model.InboundTask{
		ActivityIRI: "https://remote.com/statuses/201/quote-request",
		ObjectIRI:   "https://remote.com/statuses/201/quote-request",
		Payload:     payload,
	}

	err := svc.ProcessInboundTask(ctx, task)
	if err != nil {
		t.Fatalf("Expected manual policy processing to succeed (remaining pending), but got error: %v", err)
	}
}
