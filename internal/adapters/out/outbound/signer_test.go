package outbound_test

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sprezz/internal/adapters/out/outbound"
	inhttp "sprezz/internal/adapters/in/http"
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
	var handlerErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader := r.Header.Get("Signature")
		digestHeader := r.Header.Get("Digest")
		dateHeader := r.Header.Get("Date")

		if !strings.HasPrefix(digestHeader, "SHA-256=") {
			handlerErr = fmt.Errorf("missing or invalid digest header: %s", digestHeader)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Parse signature fields to extract the signature string
		fields := make(map[string]string)
		for _, part := range strings.Split(sigHeader, ",") {
			pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(pair) == 2 {
				fields[pair[0]] = strings.Trim(strings.TrimSpace(pair[1]), "\"")
			}
		}

		signatureBytes, err := base64.StdEncoding.DecodeString(fields["signature"])
		if err != nil {
			handlerErr = fmt.Errorf("failed to decode signature string: %w", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// FIXED: Use the new production RequestHost utility to accurately resolve
		// the canonical host value and test port removal compatibility.
		cleanHost := inhttp.RequestHost(r)

		// Rebuild the expected signing string pattern matching production
		canonical := fmt.Sprintf("(request-target): post %s\nhost: %s\ndate: %s\ndigest: %s",
			r.URL.RequestURI(), cleanHost, dateHeader, digestHeader)

		hash := sha256.Sum256([]byte(canonical))
		if err := rsa.VerifyPKCS1v15(&privKey.PublicKey, crypto.SHA256, hash[:], signatureBytes); err != nil {
			handlerErr = fmt.Errorf("cryptographic signature mismatch on remote: %w", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

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

	if handlerErr != nil {
		t.Fatalf("Remote server rejected verification payload: %v", handlerErr)
	}
}
