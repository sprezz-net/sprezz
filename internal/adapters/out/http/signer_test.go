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

	inhttp "sprezz/internal/adapters/in/http"
	outhttp "sprezz/internal/adapters/out/http"
)

type testKeys struct {
	rsaPEM        string
	edPEM         string
	rsaPublicKey *rsa.PublicKey
}

func TestForwardFederatedActivity_Success(t *testing.T) {
	// 1. Generate multi-algorithm test key materials using an isolated helper function [source: 2]
	keys, err := generateTestKeys()
	if err != nil {
		t.Fatalf("Failed to initialize dual-key test materials: %v", err)
	}

	// 2. Instantiate a thread-safe context carrier to capture async validation errors safely [source: 11]
	var handlerErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := verifyIncomingRequestSignature(r, keys.rsaPublicKey); err != nil {
			handlerErr = err
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	// 3. Dispatch activity with dual-keys using a flat entry sequence [source: 11]
	signer := outhttp.NewFederatedSignerAdapter()
	payload := []byte(`{"type":"Create","actor":"https://sprezz.net"}`)

	err = signer.ForwardFederatedActivity(
		context.Background(),
		server.URL,
		"https://sprezz.net#main-key",
		keys.rsaPEM,
		keys.edPEM,
		payload,
	)

	if err != nil {
		t.Fatalf("Expected successful outbound dispatch, got error: %v", err)
	}
	if handlerErr != nil {
		t.Fatalf("Remote server rejected verification payload: %v", handlerErr)
	}
}

// generateTestKeys encapsulates heavy standard-library x509 math away from the main test function [source: 2].
func generateTestKeys() (*testKeys, error) {
	privKeyRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	rsaPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privKeyRSA)})

	_, privKeyEd, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	edBytes, err := x509.MarshalPKCS8PrivateKey(privKeyEd)
	if err != nil {
		return nil, err
	}
	edPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: edBytes})

	return &testKeys{
		rsaPEM:        string(rsaPEM),
		edPEM:         string(edPEM),
		rsaPublicKey: &privKeyRSA.PublicKey,
	}, nil
}

// verifyIncomingRequestSignature flattens multi-layered parsing blocks down to a clean validation routine.
func verifyIncomingRequestSignature(r *http.Request, expectedPublicKey *rsa.PublicKey) error {
	sigHeader := r.Header.Get("Signature")
	digestHeader := r.Header.Get("Digest")
	dateHeader := r.Header.Get("Date")

	if !strings.HasPrefix(digestHeader, "SHA-256=") {
		return fmt.Errorf("missing or invalid digest header: %s", digestHeader)
	}

	params := make(map[string]string)
	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			params[kv[0]] = strings.Trim(strings.TrimSpace(kv[1]), "\"")
		}
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(params["signature"])
	if err != nil {
		return fmt.Errorf("failed to decode signature string: %w", err)
	}

	cleanHost := inhttp.RequestHost(r)
	canonical := fmt.Sprintf("(request-target): post %s\nhost: %s\ndate: %s\ndigest: %s",
		r.URL.RequestURI(), cleanHost, dateHeader, digestHeader)

	hash := sha256.Sum256([]byte(canonical))
	if err := rsa.VerifyPKCS1v15(expectedPublicKey, crypto.SHA256, hash[:], signatureBytes); err != nil {
		return fmt.Errorf("cryptographic signature mismatch on remote: %w", err)
	}

	return nil
}
