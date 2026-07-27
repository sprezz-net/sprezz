package http

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/ports"
)

type FederatedSignatureVerifier struct {
	storage ports.StoragePort
}

func NewFederatedSignatureVerifier(s ports.StoragePort) *FederatedSignatureVerifier {
	return &FederatedSignatureVerifier{storage: s}
}

var _ middleware.SignatureVerifier = (*FederatedSignatureVerifier)(nil)

// Verify serves as a highly flat orchestration entry point with minimal cognitive complexity.
func (v *FederatedSignatureVerifier) Verify(r *http.Request, body []byte) error {
	ctx := r.Context()
	sigHeader := r.Header.Get("Signature")
	if sigHeader == "" {
		return fmt.Errorf("missing signature header")
	}

	params := parseSignatureHeader(sigHeader)
	keyID, signatureBase64, headersList := params["keyId"], params["signature"], params["headers"]
	if keyID == "" || signatureBase64 == "" {
		return fmt.Errorf("malformed signature header attributes")
	}

	signingString, err := constructSigningString(r, headersList)
	if err != nil {
		return fmt.Errorf("failed to build signing payload: %w", err)
	}

	// Pass 'r' directly into the helper signature so it is available in scope
	publicPEM, err := v.resolvePublicKeyPEM(ctx, r, keyID, r.Header.Get("Date"))
	if err != nil {
		return fmt.Errorf("failed to resolve verification key: %w", err)
	}

	// Extract and decode the PEM block string back to a concrete *rsa.PublicKey object
	rsaPubKey, err := decodePEMToRSAPublicKey(publicPEM)
	if err != nil {
		return fmt.Errorf("invalid public key structure: %w", err)
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return fmt.Errorf("failed to decode base64 signature: %w", err)
	}

	hashed := sha256.Sum256([]byte(signingString))
	if err := rsa.VerifyPKCS1v15(rsaPubKey, crypto.SHA256, hashed[:], signatureBytes); err != nil {
		return fmt.Errorf("cryptographic validation failed: signature mismatch")
	}

	return nil
}

// resolvePublicKeyPEM routes key location checks dynamically based on domain context boundaries.
func (v *FederatedSignatureVerifier) resolvePublicKeyPEM(ctx context.Context, r *http.Request, keyID, dateHeader string) (string, error) {
	requestTime, err := time.Parse(time.RFC1123, dateHeader)
	if err != nil {
		requestTime = time.Now().UTC()
	}

	if !strings.Contains(keyID, r.Host) {
		return "", fmt.Errorf("remote verification fetch loop not implemented in sandbox")
	}

	keyType := "RSA"
	if strings.HasSuffix(keyID, "#ed25519-key") {
		keyType = "Ed25519"
	}
	actorIRI := strings.Split(keyID, "#")[0]

	historicalKey, err := v.storage.GetHistoricalKey(ctx, actorIRI, keyType, requestTime)
	if err == nil {
		v.assertActorIdentityHeader(r, actorIRI)
		return historicalKey, nil
	}

	username := strings.Split(actorIRI, "/")[len(strings.Split(actorIRI, "/"))-1]
	_, dualKeys, fallbackErr := v.storage.GetActorCredentials(ctx, 0, username)
	if fallbackErr != nil {
		return "", fmt.Errorf("failed to locate active credentials: %w", fallbackErr)
	}

	v.assertActorIdentityHeader(r, actorIRI)
	return dualKeys.PrivateKeyRSAPEM, nil
}

// assertActorIdentityHeader notifies downstream middleware of verified identity strings [source: 3].
func (v *FederatedSignatureVerifier) assertActorIdentityHeader(r *http.Request, actorIRI string) {
	if r != nil {
		r.Header.Set("X-Actor-IRI", actorIRI) // Sets identity directly for the middleware
	}
}

// Isolated helper function to decode PEM text directly back to operational RSA object values cleanly
func decodePEMToRSAPublicKey(publicPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM bytes")
	}

	// Try parsing it as a PKCS1 Private Key string first if it's a local credential block
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return &privKey.PublicKey, nil
	}

	// Otherwise, process standard PKIX public elements natively
	pubKey, parseErr := x509.ParsePKIXPublicKey(block.Bytes)
	if parseErr != nil {
		return nil, parseErr
	}

	rsaPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("decoded public key block is not a valid RSA type")
	}
	return rsaPubKey, nil
}

func parseSignatureHeader(header string) map[string]string {
	result := make(map[string]string)
	pairs := strings.Split(header, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = strings.Trim(kv[1], `"`)
		}
	}
	if result["headers"] == "" {
		result["headers"] = "date"
	}
	return result
}

func constructSigningString(r *http.Request, headersList string) (string, error) {
	var lines []string
	keys := strings.Split(headersList, " ")
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		switch key {
		case "(request-target)":
			lines = append(lines, fmt.Sprintf("(request-target): %s %s", strings.ToLower(r.Method), r.URL.RequestURI()))
		case "host":
			lines = append(lines, fmt.Sprintf("host: %s", r.Host))
		default:
			val := r.Header.Get(key)
			if val == "" {
				return "", fmt.Errorf("required signing header %q missing from request", key)
			}
			lines = append(lines, fmt.Sprintf("%s: %s", key, val))
		}
	}
	return strings.Join(lines, "\n"), nil
}
