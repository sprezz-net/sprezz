package outbound_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sprezz/internal/adapters/out/outbound"
)

func TestForwardFederatedActivity_Success(t *testing.T) {
	// 1. Generate RSA key pair for testing
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate test RSA key: %v", err)
	}

	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})

	// 2. Setup mock target inbox HTTP server
	receivedSignature := ""
	receivedDigest := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("Signature")
		receivedDigest = r.Header.Get("Digest")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	// 3. Dispatch activity
	signer := outbound.NewFederatedSignerAdapter()
	payload := []byte(`{"type":"Create","actor":"https://sprezz.net/actors/alice"}`)

	err = signer.ForwardFederatedActivity(
		context.Background(),
		server.URL,
		"https://sprezz.net/actors/alice#main-key",
		string(privKeyPEM),
		payload,
	)

	if err != nil {
		t.Fatalf("Expected successful outbound dispatch, got error: %v", err)
	}

	if !strings.HasPrefix(receivedDigest, "SHA-256=") {
		t.Errorf("Expected SHA-256 Digest header, got: %s", receivedDigest)
	}

	if !strings.Contains(receivedSignature, `keyId="https://sprezz.net/actors/alice#main-key"`) {
		t.Errorf("Expected keyId in Signature header, got: %s", receivedSignature)
	}
	if !strings.Contains(receivedSignature, `algorithm="rsa-sha256"`) {
		t.Errorf("Expected rsa-sha256 algorithm in Signature header, got: %s", receivedSignature)
	}
}
