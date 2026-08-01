package http_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/pkg/integrityproof"
	"sprezz/internal/pkg/jcs"
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

	signedDoc, err := integrityproof.SignDataIntegrityProof(docMap, priv, keyID, created)
	if err != nil {
		t.Fatal(err)
	}

	signedDocBytes, err := jcs.Format(signedDoc)
	if err != nil {
		signedDocBytes, err = json.Marshal(signedDoc)
		if err != nil {
			t.Fatal(err)
		}
	}

	// 4. Setup request
	request := httptest.NewRequest(http.MethodPost, "/inbox", strings.NewReader(string(signedDocBytes)))
	request.Host = "local.example"
	request.Header.Set("Date", "Fri, 24 Feb 2023 23:36:38 GMT")

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
