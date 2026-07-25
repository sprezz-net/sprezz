package http_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	inhttp "sprezz/internal/adapters/in/http"
)

type testKeyResolver struct {
	key *rsa.PublicKey
}

func (r testKeyResolver) ResolvePublicKey(string) (*rsa.PublicKey, error) {
	return r.key, nil
}

func TestSignatureVerifierAcceptsValidRequest(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"id":"https://remote.example"}`)
	digestBytes := sha256.Sum256(body)
	digest := base64.StdEncoding.EncodeToString(digestBytes[:])
	date := time.Now().UTC().Format(http.TimeFormat)

	// Use a relative target path to ensure request.URL.RequestURI()
	// evaluates exactly to "/inbox/alice" just like it does under live production traffic.
	request := httptest.NewRequest(http.MethodPost, "/inbox/alice", strings.NewReader(string(body)))
	request.Host = "local.example"

	request.Header.Set("Date", date)
	request.Header.Set("Digest", "SHA-256="+digest)

	expectedHost := "local.example"
	keyID := "https://remote.example"

	canonical := fmt.Sprintf("(request-target): post %s\nhost: %s\ndate: %s\ndigest: SHA-256=%s",
		request.URL.RequestURI(), expectedHost, date, digest)

	canonicalHash := sha256.Sum256([]byte(canonical))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, canonicalHash[:])
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Signature", fmt.Sprintf("keyId=\"%s\",algorithm=\"rsa-sha256\",headers=\"(request-target) host date digest\",signature=\"%s\"", keyID, base64.StdEncoding.EncodeToString(signature)))

	verifier := inhttp.NewSignatureVerifier(testKeyResolver{key: &privateKey.PublicKey})

	// 1. Verify a perfectly valid cryptographically signed request pass
	if err := verifier.Verify(request, body); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	// 2. Assert that the verifier correctly injected the verified keyId into the fallback header for the middleware
	actorIRI := request.Header.Get("X-Actor-IRI")
	if actorIRI != keyID {
		t.Errorf("Expected X-Actor-IRI header fallback token to match %q, got %q", keyID, actorIRI)
	}

	// 3. Verify that tampering with the payload body bytes breaks digest matching immediately
	if err := verifier.Verify(request, append(body, 'x')); err == nil {
		t.Fatal("Tampered request body payload was incorrectly accepted by the verifier")
	}
}
