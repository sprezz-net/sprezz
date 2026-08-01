package http_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/domain/service"
	"sprezz/internal/pkg/cryptoutil"

	"github.com/gojuno/minimock/v3"
)

func TestSignatureVerifier_ObjectIntegrityProof_FEP8b32(t *testing.T) {
	mc := minimock.NewController(t)

	// 1. Generate Ed25519 Key pair
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Marshal private key to PEM as stored in our credential system
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	// 3. Prepare doc and sign it
	docMap := map[string]interface{}{
		"@context": []interface{}{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/data-integrity/v2",
		},
		"id":    "https://local.example/activities/1",
		"type":  "Create",
		"actor": "https://local.example/users/alice",
		"object": map[string]interface{}{
			"id":      "https://local.example/objects/1",
			"type":    "Note",
			"content": "Hello verified world",
		},
	}

	keyID := "https://local.example/users/alice#ed25519-key"
	created := "2023-02-24T23:36:38Z"

	signedDoc, err := cryptoutil.SignDataIntegrityProof(docMap, priv, keyID, created)
	if err != nil {
		t.Fatal(err)
	}

	signedDocBytes, err := cryptoutil.FormatJCS(signedDoc)
	if err != nil {
		signedDocBytes, err = json.Marshal(signedDoc)
		if err != nil {
			t.Fatal(err)
		}
	}

	// 4. Setup request
	request := httptest.NewRequest(http.MethodPost, "/inbox", strings.NewReader(string(signedDocBytes)))
	request.Host = "local.example"
	request.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))

	// 5. Mock storage to return our key PEM
	mockStorage := portmock.NewStoragePortMock(mc)
	mockStorage.GetHistoricalKeyMock.Inspect(func(ctx context.Context, actorIRI string, keyType string, signedAt time.Time) {
		if actorIRI != "https://local.example/users/alice" {
			t.Errorf("Expected actorIRI query %q, got %q", "https://local.example/users/alice", actorIRI)
		}
		if keyType != "Ed25519" {
			t.Errorf("Expected keyType %q, got %q", "Ed25519", keyType)
		}
	}).Return(privPEM, nil)

	verifier := inhttp.NewFederatedSignatureVerifier(mockStorage)

	// 6. Verify (should succeed purely based on the FEP-8b32 object integrity proof, even without "Signature" header!)
	if err := verifier.Verify(request, signedDocBytes); err != nil {
		t.Fatalf("Verifier rejected FEP-8b32 proof signature path: %v", err)
	}

	actorIRI := request.Header.Get("X-Actor-IRI")
	expectedIRI := "https://local.example/users/alice"
	if actorIRI != expectedIRI {
		t.Errorf("Expected extracted X-Actor-IRI %q, got %q", expectedIRI, actorIRI)
	}
}

func TestSignatureVerifier_FEP8c13_AuthorAndForwardingProof(t *testing.T) {
	mc := minimock.NewController(t)

	// 1. Generate keys for Author (Alice) and Forwarder (Bob)
	_, authorPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, forwarderPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Marshal private keys to PEM
	authorPKCS8, err := x509.MarshalPKCS8PrivateKey(authorPriv)
	if err != nil {
		t.Fatal(err)
	}
	authorPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: authorPKCS8}))

	forwarderPKCS8, err := x509.MarshalPKCS8PrivateKey(forwarderPriv)
	if err != nil {
		t.Fatal(err)
	}
	forwarderPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: forwarderPKCS8}))

	// 3. Prepare document map
	docMap := map[string]interface{}{
		"@context": []interface{}{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/data-integrity/v1",
			"https://w3id.org/fep/8c13",
		},
		"id":             "https://local.example/activities/98765",
		"type":           "Create",
		"actor":          "https://local.example/users/alice",
		"contextHistory": "https://local.example/history/12345",
		"to":             []interface{}{"https://local.example/users/alice/followers"},
		"cc":             []interface{}{"https://local.example/users/alice"},
		"object": map[string]interface{}{
			"id":             "https://local.example/posts/98765",
			"type":           "Note",
			"contextHistory": "https://local.example/history/12345",
			"content":        "Hi followers",
		},
	}

	authorKeyID := "https://local.example/users/alice#ed25519-key"
	forwarderKeyID := "https://local.example/users/bob#ed25519-key"

	// 4. Sign Author Proof
	signedDoc, err := service.SignAuthorProof(docMap, authorPriv, authorKeyID, "2026-01-08T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}

	// 5. Sign Forwarding Proof
	forwardedDoc, err := service.SignForwardingProof(signedDoc, forwarderPriv, forwarderKeyID, "2026-01-08T12:01:00Z")
	if err != nil {
		t.Fatal(err)
	}

	forwardedBytes, err := json.Marshal(forwardedDoc)
	if err != nil {
		t.Fatal(err)
	}

	// 6. Setup request
	request := httptest.NewRequest(http.MethodPost, "/inbox", strings.NewReader(string(forwardedBytes)))
	request.Host = "local.example"
	request.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))

	// 7. Mock storage to return PEM keys
	mockStorage := portmock.NewStoragePortMock(mc)
	mockStorage.GetHistoricalKeyMock.Set(func(ctx context.Context, actorIRI string, keyType string, signedAt time.Time) (string, error) {
		if actorIRI == "https://local.example/users/alice" {
			return authorPEM, nil
		}
		if actorIRI == "https://local.example/users/bob" {
			return forwarderPEM, nil
		}
		return "", fmt.Errorf("key not found")
	})

	verifier := inhttp.NewFederatedSignatureVerifier(mockStorage)

	// 8. Verify
	if err := verifier.Verify(request, forwardedBytes); err != nil {
		t.Fatalf("Verifier rejected FEP-8c13 author & forwarding proof path: %v", err)
	}

	actorIRI := request.Header.Get("X-Actor-IRI")
	expectedIRI := "https://local.example/users/alice"
	if actorIRI != expectedIRI {
		t.Errorf("Expected extracted X-Actor-IRI %q, got %q", expectedIRI, actorIRI)
	}
}
