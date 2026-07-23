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
	body := []byte(`{"id":"https://remote.example/activities/1"}`)
	digestBytes := sha256.Sum256(body)
	digest := base64.StdEncoding.EncodeToString(digestBytes[:])
	date := time.Now().UTC().Format(http.TimeFormat)
	request := httptest.NewRequest(http.MethodPost, "https://local.example/inbox/alice", strings.NewReader(string(body)))
	request.Host = "local.example"
	request.Header.Set("Date", date)
	request.Header.Set("Digest", "SHA-256="+digest)
	canonical := fmt.Sprintf("(request-target): post %s\nhost: %s\ndate: %s\ndigest: SHA-256=%s", request.URL.RequestURI(), request.Host, date, digest)
	canonicalHash := sha256.Sum256([]byte(canonical))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, canonicalHash[:])
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Signature", fmt.Sprintf("keyId=\"https://remote.example/keys/1\",algorithm=\"rsa-sha256\",headers=\"(request-target) host date digest\",signature=\"%s\"", base64.StdEncoding.EncodeToString(signature)))

	verifier := inhttp.NewSignatureVerifier(testKeyResolver{key: &privateKey.PublicKey})
	if err := verifier.Verify(request, body); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if err := verifier.Verify(request, append(body, 'x')); err == nil {
		t.Fatal("tampered request was accepted")
	}
}
