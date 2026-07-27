package http_test

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/domain/port/portmock"
)

func TestSignatureVerifier_MultiAlgorithm_TableDriven(t *testing.T) {
	// 1. Setup RSA Test Key Material
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaPriv)}
	rsaPrivateKeyPEM := string(pem.EncodeToMemory(rsaBlock))

	// 2. Setup Ed25519 Test Key Material
	_, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edBytes, err := x509.MarshalPKCS8PrivateKey(edPriv)
	if err != nil {
		t.Fatal(err)
	}
	edPrivateKeyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: edBytes}))

	// 3. Define Table-Driven Scenarios
	tests := []struct {
		name         string
		keyType      string
		keyIDSuffix  string
		algorithmStr string
		privatePEM   string
		signFn       func(canonical []byte) []byte
	}{
		{
			name:         "Validate Local Legacy RSA Pipeline",
			keyType:      "RSA",
			keyIDSuffix:  "#main-key",
			algorithmStr: "rsa-sha256",
			privatePEM:   rsaPrivateKeyPEM,
			signFn: func(canonical []byte) []byte {
				hashed := sha256.Sum256(canonical)
				sig, _ := rsa.SignPKCS1v15(rand.Reader, rsaPriv, crypto.SHA256, hashed[:])
				return sig
			},
		},
		{
			name:         "Validate Modern Ed25519 Pipeline",
			keyType:      "Ed25519",
			keyIDSuffix:  "#ed25519-key",
			algorithmStr: "ed25519-sha256",
			privatePEM:   edPrivateKeyPEM,
			signFn: func(canonical []byte) []byte {
				return ed25519.Sign(edPriv, canonical)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)

			body := []byte(`{"id":"https://remote.example"}`)
			date := time.Now().UTC().Format(http.TimeFormat)

			request := httptest.NewRequest(http.MethodPost, "/inbox/alice", strings.NewReader(string(body)))
			request.Host = "local.example"
			request.Header.Set("Date", date)

			keyID := "https://local.example" + tt.keyIDSuffix
			canonical := fmt.Sprintf("(request-target): post %s\nhost: %s\ndate: %s",
				request.URL.RequestURI(), request.Host, date)

			signatureBytes := tt.signFn([]byte(canonical))
			signatureBase64 := base64.StdEncoding.EncodeToString(signatureBytes)

			request.Header.Set("Signature", fmt.Sprintf(
				"keyId=\"%s\",algorithm=\"%s\",headers=\"(request-target) host date\",signature=\"%s\"",
				keyID, tt.algorithmStr, signatureBase64,
			))

			mockStorage := portmock.NewStoragePortMock(mc)
			mockStorage.GetHistoricalKeyMock.Inspect(func(ctx context.Context, actorIRI string, keyType string, signedAt time.Time) {
				if keyType != tt.keyType {
					t.Errorf("Expected key type query %q, got %q", tt.keyType, keyType)
				}
			}).Return(tt.privatePEM, nil)

			verifier := inhttp.NewFederatedSignatureVerifier(mockStorage)

			if err := verifier.Verify(request, body); err != nil {
				t.Fatalf("Verifier rejected valid %s signature path: %v", tt.keyType, err)
			}

			actorIRI := request.Header.Get("X-Actor-IRI")
			expectedIRI := "https://local.example"
			if actorIRI != expectedIRI {
				t.Errorf("Expected extracted X-Actor-IRI %q, got %q", expectedIRI, actorIRI)
			}
		})
	}
}
