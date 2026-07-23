package outbound

import (
	"bytes"
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
	"time"
)

type FederatedSignerAdapter struct {
	client *http.Client
}

func NewFederatedSignerAdapter() *FederatedSignerAdapter {
	return &FederatedSignerAdapter{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *FederatedSignerAdapter) ForwardFederatedActivity(ctx context.Context, targetInbox, actorKeyID, privateKeyPEM string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", targetInbox, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("User-Agent", "Sprezz-Hex-QuadStore/2.0")

	hasher := sha256.New()
	hasher.Write(payload)
	digestBase64 := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	req.Header.Set("Digest", fmt.Sprintf("SHA-256=%s", digestBase64))

	req.Header.Set("Host", req.URL.Host)
	dateStr := time.Now().UTC().Format(http.TimeFormat)
	req.Header.Set("Date", dateStr)

	signingString := fmt.Sprintf("(request-target): post %s\nhost: %s\ndate: %s\ndigest: SHA-256=%s",
		req.URL.Path, req.URL.Host, dateStr, digestBase64)

	signature, err := signString(signingString, privateKeyPEM)
	if err != nil {
		return fmt.Errorf("failed to sign outbound request headers: %w", err)
	}

	sigHeaderVal := fmt.Sprintf("keyId=\"%s\",algorithm=\"rsa-sha256\",headers=\"(request-target) host date digest\",signature=\"%s\"",
		actorKeyID, signature)
	req.Header.Set("Signature", sigHeaderVal)

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("remote network endpoint refused activity delivery: status code %d", resp.StatusCode)
	}

	return nil
}

func signString(message, privateKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to parse raw identity key block format")
	}

	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		if parsedKey, err8 := x509.ParsePKCS8PrivateKey(block.Bytes); err8 == nil {
			if rsaKey, ok := parsedKey.(*rsa.PrivateKey); ok {
				privKey = rsaKey
			} else {
				return "", fmt.Errorf("key is not RSA private key: %w", err)
			}
		} else {
			return "", err
		}
	}

	msgHash := sha256.Sum256([]byte(message))
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, msgHash[:])
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(sigBytes), nil
}
