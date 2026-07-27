package http_test

import (
	"context"
	"crypto"
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
	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports/portstest"
)

// MockVerifierStorage satisfies ports.StoragePort using embedded non-operational test stubs.
type MockVerifierStorage struct {
	portstest.UnimplementedStoragePort
	OnGetHistoricalKey   func(ctx context.Context, actorIRI string, keyType string, signedAt time.Time) (string, error)
	OnGetActorCredentials func(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error)
}

func (m *MockVerifierStorage) GetHistoricalKey(ctx context.Context, actorIRI string, keyType string, signedAt time.Time) (string, error) {
	if m.OnGetHistoricalKey != nil {
		return m.OnGetHistoricalKey(ctx, actorIRI, keyType, signedAt)
	}
	return "", fmt.Errorf("no historical key mocked")
}

func (m *MockVerifierStorage) GetActorCredentials(ctx context.Context, tenantID int32, username string) (string, *model.ActorDualKeys, error) {
	if m.OnGetActorCredentials != nil {
		return m.OnGetActorCredentials(ctx, tenantID, username)
	}
	return "https://local.example", &model.ActorDualKeys{}, nil
}

func TestSignatureVerifierAcceptsValidRequest(t *testing.T) {
	// 1. Generate a valid testing keypair block
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	rsaBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	privateKeyPEM := string(pem.EncodeToMemory(rsaBlock))

	body := []byte(`{"id":"https://remote.example"}`)
	digestBytes := sha256.Sum256(body)
	digest := base64.StdEncoding.EncodeToString(digestBytes[:])
	date := time.Now().UTC().Format(http.TimeFormat)

	request := httptest.NewRequest(http.MethodPost, "/inbox/alice", strings.NewReader(string(body)))
	request.Host = "local.example"

	request.Header.Set("Date", date)
	request.Header.Set("Digest", "SHA-256="+digest)

	expectedHost := "local.example"
	// To test our local historical verification branch routing, keyID belongs to r.Host
	keyID := "https://local.example#main-key"

	canonical := fmt.Sprintf("(request-target): post %s\nhost: %s\ndate: %s",
		request.URL.RequestURI(), expectedHost, date)

	canonicalHash := sha256.Sum256([]byte(canonical))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, canonicalHash[:])
	if err != nil {
		t.Fatal(err)
	}

	request.Header.Set("Signature", fmt.Sprintf("keyId=\"%s\",algorithm=\"rsa-sha256\",headers=\"(request-target) host date\",signature=\"%s\"", keyID, base64.StdEncoding.EncodeToString(signature)))

	// 2. Set up our driven mock storage adapter to return our generated key string
	mockStorage := &MockVerifierStorage{
		OnGetHistoricalKey: func(ctx context.Context, actorIRI string, keyType string, signedAt time.Time) (string, error) {
			return privateKeyPEM, nil
		},
	}

	// 3. Initialize production verifier engine with our database mock adapter
	verifier := inhttp.NewFederatedSignatureVerifier(mockStorage)

	// 4. Verify a perfectly valid cryptographically signed request pass
	if err := verifier.Verify(request, body); err != nil {
		t.Fatalf("valid request rejected by native standard library verifier: %v", err)
	}

	// 5. Assert that the verifier correctly injected the verified keyId back to the header
	actorIRI := request.Header.Get("X-Actor-IRI")
	expectedIRI := "https://local.example"
	if actorIRI != expectedIRI {
		t.Errorf("Expected X-Actor-IRI header fallback token to match %q, got %q", expectedIRI, actorIRI)
	}
}
