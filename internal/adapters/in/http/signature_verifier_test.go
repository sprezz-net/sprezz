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

	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/domain/port/portmock"

	"github.com/gojuno/minimock/v3"
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

			verifier := inhttp.NewFederatedSignatureVerifier(mockStorage, nil)

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

func TestSignatureVerifier_ActorSpoofingPrevention_MatchingDomains(t *testing.T) {
	// Setup RSA Test Key Material
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaPriv)}
	rsaPrivateKeyPEM := string(pem.EncodeToMemory(rsaBlock))

	mc := minimock.NewController(t)

	actorIRI := "https://local.example/actor/alice"
	keyID := "https://local.example/actor/alice#main-key"

	body := fmt.Appendf(nil, `{"type":"Create","actor":"%s"}`, actorIRI)
	date := time.Now().UTC().Format(http.TimeFormat)

	request := httptest.NewRequest(http.MethodPost, "/inbox", strings.NewReader(string(body)))
	request.Host = "local.example"
	request.Header.Set("Date", date)

	canonical := fmt.Sprintf("(request-target): post %s\nhost: %s\ndate: %s",
		request.URL.RequestURI(), request.Host, date)

	hashed := sha256.Sum256([]byte(canonical))
	signatureBytes, _ := rsa.SignPKCS1v15(rand.Reader, rsaPriv, crypto.SHA256, hashed[:])
	signatureBase64 := base64.StdEncoding.EncodeToString(signatureBytes)

	request.Header.Set("Signature", fmt.Sprintf(
		"keyId=\"%s\",algorithm=\"rsa-sha256\",headers=\"(request-target) host date\",signature=\"%s\"",
		keyID, signatureBase64,
	))

	mockStorage := portmock.NewStoragePortMock(mc)
	mockStorage.GetHistoricalKeyMock.Return(rsaPrivateKeyPEM, nil)

	verifier := inhttp.NewFederatedSignatureVerifier(mockStorage, nil)

	err = verifier.Verify(request, body)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestSignatureVerifier_ActorSpoofingPrevention_MismatchedDomains(t *testing.T) {
	// Setup RSA Test Key Material
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	mc := minimock.NewController(t)

	actorIRI := "https://high-profile.example/actor/target"
	keyID := "https://attacker.example/actor/malicious#main-key"

	body := fmt.Appendf(nil, `{"type":"Create","actor":"%s"}`, actorIRI)
	date := time.Now().UTC().Format(http.TimeFormat)

	request := httptest.NewRequest(http.MethodPost, "/inbox", strings.NewReader(string(body)))
	request.Host = "local.example"
	request.Header.Set("Date", date)

	canonical := fmt.Sprintf("(request-target): post %s\nhost: %s\ndate: %s",
		request.URL.RequestURI(), request.Host, date)

	hashed := sha256.Sum256([]byte(canonical))
	signatureBytes, _ := rsa.SignPKCS1v15(rand.Reader, rsaPriv, crypto.SHA256, hashed[:])
	signatureBase64 := base64.StdEncoding.EncodeToString(signatureBytes)

	request.Header.Set("Signature", fmt.Sprintf(
		"keyId=\"%s\",algorithm=\"rsa-sha256\",headers=\"(request-target) host date\",signature=\"%s\"",
		keyID, signatureBase64,
	))

	mockStorage := portmock.NewStoragePortMock(mc)
	verifier := inhttp.NewFederatedSignatureVerifier(mockStorage, nil)

	err = verifier.Verify(request, body)
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	expectedError := "security violation: signature keyId domain"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing %q, got %q", expectedError, err.Error())
	}
}

func TestSignatureVerifier_ClockSkew_Rejection(t *testing.T) {
	mc := minimock.NewController(t)

	body := []byte(`{"id":"https://remote.example"}`)

	// Create a Date header that is 1 hour in the past (clock skew is > 5 mins)
	staleDate := time.Now().Add(-1 * time.Hour).UTC().Format(http.TimeFormat)

	request := httptest.NewRequest(http.MethodPost, "/inbox/alice", strings.NewReader(string(body)))
	request.Host = "local.example"
	request.Header.Set("Date", staleDate)

	mockStorage := portmock.NewStoragePortMock(mc)
	verifier := inhttp.NewFederatedSignatureVerifier(mockStorage, nil)

	err := verifier.Verify(request, body)
	if err == nil {
		t.Fatalf("Expected error due to clock skew, got nil")
	}
	expectedError := "clock drift exceeds 5-minute maximum limit"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing %q, got: %v", expectedError, err)
	}
}

func TestSignatureVerifier_PublicKeyCacheExpiryFallback(t *testing.T) {
	mc := minimock.NewController(t)

	// 1. Setup Outdated RSA Key Material (stored in cache)
	outdatedPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	outdatedBytes, err := x509.MarshalPKIXPublicKey(&outdatedPriv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	outdatedBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: outdatedBytes}
	outdatedPublicKeyPEM := string(pem.EncodeToMemory(outdatedBlock))

	// 2. Setup Fresh RSA Key Material (rotated on remote)
	freshPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	freshBytes, err := x509.MarshalPKIXPublicKey(&freshPriv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	freshBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: freshBytes}
	freshPublicKeyPEM := string(pem.EncodeToMemory(freshBlock))

	body := []byte(`{"id":"https://remote.example"}`)
	date := time.Now().UTC().Format(http.TimeFormat)

	request := httptest.NewRequest(http.MethodPost, "/inbox/alice", strings.NewReader(string(body)))
	request.Host = "local.example"
	request.Header.Set("Date", date)

	// Sign the request with the FRESH private key
	keyID := "https://remote.example/users/alice#main-key"
	canonical := fmt.Sprintf("(request-target): post %s\nhost: %s\ndate: %s",
		request.URL.RequestURI(), request.Host, date)

	hashed := sha256.Sum256([]byte(canonical))
	signatureBytes, _ := rsa.SignPKCS1v15(rand.Reader, freshPriv, crypto.SHA256, hashed[:])
	signatureBase64 := base64.StdEncoding.EncodeToString(signatureBytes)

	request.Header.Set("Signature", fmt.Sprintf(
		"keyId=\"%s\",algorithm=\"rsa-sha256\",headers=\"(request-target) host date\",signature=\"%s\"",
		keyID, signatureBase64,
	))

	// 3. Mock Storage Port
	mockStorage := portmock.NewStoragePortMock(mc)
	// Return the outdated public key on first call (causes verifySignature to fail)
	mockStorage.GetHistoricalKeyMock.Return(outdatedPublicKeyPEM, nil)
	// Expect cache invalidation
	mockStorage.DeleteActorKeyHistoryMock.Expect(context.Background(), "https://remote.example/users/alice").Return(nil)
	// Expect fresh public key archival
	mockStorage.ArchiveKeyHistoryMock.Inspect(func(ctx context.Context, actorIRI string, keyType string, publicKeyPEM string, validFrom time.Time, validTo time.Time) {
		if actorIRI != "https://remote.example/users/alice" {
			t.Errorf("Expected actorIRI %q, got %q", "https://remote.example/users/alice", actorIRI)
		}
		if keyType != "RSA" {
			t.Errorf("Expected keyType %q, got %q", "RSA", keyType)
		}
		if publicKeyPEM != freshPublicKeyPEM {
			t.Errorf("Expected public key PEM %q, got %q", freshPublicKeyPEM, publicKeyPEM)
		}
	}).Return(nil)

	// 4. Mock Remote Fetcher
	mockFetcher := portmock.NewRemoteFetcherMock(mc)
	remoteActorJSON := fmt.Sprintf(`{
		"publicKey": {
			"id": "%s",
			"owner": "https://remote.example/users/alice",
			"publicKeyPem": %q
		}
	}`, keyID, freshPublicKeyPEM)

	mockFetcher.FetchSignedMock.Expect(context.Background(), "https://remote.example/users/alice", "", "", "").Return([]byte(remoteActorJSON), nil)

	// 5. Instantiate and verify fallback block triggers perfectly
	verifier := inhttp.NewFederatedSignatureVerifier(mockStorage, mockFetcher)

	if err := verifier.Verify(request, body); err != nil {
		t.Fatalf("Verifier rejected rotated signature fallback path: %v", err)
	}

	actorIRI := request.Header.Get("X-Actor-IRI")
	expectedIRI := "https://remote.example/users/alice"
	if actorIRI != expectedIRI {
		t.Errorf("Expected extracted X-Actor-IRI %q, got %q", expectedIRI, actorIRI)
	}
}
