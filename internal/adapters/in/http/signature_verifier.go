package http

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/domain/port"
)

type FederatedSignatureVerifier struct {
	storage port.StoragePort
	fetcher port.RemoteFetcher
}

func NewFederatedSignatureVerifier(s port.StoragePort, f port.RemoteFetcher) *FederatedSignatureVerifier {
	return &FederatedSignatureVerifier{storage: s, fetcher: f}
}

var _ middleware.SignatureVerifier = (*FederatedSignatureVerifier)(nil)

func (v *FederatedSignatureVerifier) validateClockSkew(r *http.Request) error {
	dateHeader := r.Header.Get("Date")
	if dateHeader == "" {
		return nil
	}

	requestTime, err := time.Parse(time.RFC1123, dateHeader)
	if err != nil {
		return fmt.Errorf("invalid Date header format: %w", err)
	}

	drift := time.Since(requestTime)
	if drift < 0 {
		drift = -drift
	}

	if drift > 5*time.Minute {
		return fmt.Errorf("http signature verification failed: Date header clock drift exceeds 5-minute maximum limit")
	}

	return nil
}

// Verify serves as a highly flat orchestration entry point with minimal cognitive complexity.
func (v *FederatedSignatureVerifier) Verify(r *http.Request, body []byte) error {
	ctx := r.Context()

	if err := v.validateClockSkew(r); err != nil {
		return err
	}

	// Try FEP-8b32 Object Integrity Proof verification first
	if found, err := v.CheckAndVerifyIntegrityProof(r, body); found {
		return err
	}

	sigHeader := r.Header.Get("Signature")
	if sigHeader == "" {
		return fmt.Errorf("missing signature header")
	}

	params := parseSignatureHeader(sigHeader)
	keyID, signatureBase64, headersList := params["keyId"], params["signature"], params["headers"]
	if keyID == "" || signatureBase64 == "" {
		return fmt.Errorf("malformed signature header attributes")
	}

	if err := validateActorDomain(r.Method, body, keyID); err != nil {
		return err
	}

	signingString, err := constructSigningString(r, headersList)
	if err != nil {
		return fmt.Errorf("failed to build signing payload: %w", err)
	}

	// Pass 'r' directly into the helper signature so it is available in scope
	publicPEM, wasCached, err := v.resolvePublicKeyPEM(ctx, r, keyID, r.Header.Get("Date"))
	if err != nil {
		return fmt.Errorf("failed to resolve verification key: %w", err)
	}

	// Dynamically decode the PEM block into a generic public key interface
	pubKey, err := decodePEMToPublicKey(publicPEM)
	if err != nil {
		return fmt.Errorf("invalid public key structure: %w", err)
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return fmt.Errorf("failed to decode base64 signature: %w", err)
	}

	actorIRI := strings.Split(keyID, "#")[0]
	keyType := "RSA"
	if strings.HasSuffix(keyID, "#ed25519-key") {
		keyType = "Ed25519"
	}

	if err := verifySignature(pubKey, signingString, signatureBytes); err != nil {
		// If verification fails against a cached key, trigger fallback cache invalidation & retry exactly once
		if wasCached && v.tryFallbackRetry(ctx, actorIRI, keyType, signingString, signatureBytes) {
			return nil
		}
		return err
	}

	return nil
}

func (v *FederatedSignatureVerifier) tryFallbackRetry(ctx context.Context, actorIRI, keyType, signingString string, signatureBytes []byte) bool {
	// Invalidate local cache entry
	_ = v.storage.DeleteActorKeyHistory(ctx, actorIRI)

	// Execute exactly one fresh, outbound key de-referencing fetch
	freshPEM, fetchErr := v.fetchFreshPublicKeyPEM(ctx, actorIRI)
	if fetchErr != nil {
		return false
	}

	freshPubKey, decodeErr := decodePEMToPublicKey(freshPEM)
	if decodeErr != nil {
		return false
	}

	// Archive the fresh key in local history
	now := time.Now()
	_ = v.storage.ArchiveKeyHistory(ctx, actorIRI, keyType, freshPEM, now.Add(-24*time.Hour), now.Add(100*365*24*time.Hour))

	// Retry verification exactly once
	return verifySignature(freshPubKey, signingString, signatureBytes) == nil
}

// resolvePublicKeyPEM routes key location checks dynamically based on domain context boundaries.
func (v *FederatedSignatureVerifier) resolvePublicKeyPEM(ctx context.Context, r *http.Request, keyID, dateHeader string) (string, bool, error) {
	requestTime, err := time.Parse(time.RFC1123, dateHeader)
	if err != nil {
		requestTime = time.Now().UTC()
	}

	keyType := "RSA"
	if strings.HasSuffix(keyID, "#ed25519-key") {
		keyType = "Ed25519"
	}
	actorIRI := strings.Split(keyID, "#")[0]

	// 1. Try local historical cache first
	historicalKey, err := v.storage.GetHistoricalKey(ctx, actorIRI, keyType, requestTime)
	if err == nil {
		v.assertActorIdentityHeader(r, actorIRI)
		return historicalKey, true, nil
	}

	// 2. Try local actor credentials next if the key is on our own host
	if strings.Contains(keyID, r.Host) {
		username := strings.Split(actorIRI, "/")[len(strings.Split(actorIRI, "/"))-1]
		_, dualKeys, fallbackErr := v.storage.GetActorCredentials(ctx, 0, username)
		if fallbackErr == nil {
			v.assertActorIdentityHeader(r, actorIRI)
			if keyType == "Ed25519" {
				return dualKeys.PrivateKeyEd25519PEM, false, nil
			}
			return dualKeys.PrivateKeyRSAPEM, false, nil
		}
	}

	// 3. For remote actors, query outbound key de-referencing fetch if we have a fetcher configured
	if v.fetcher != nil {
		freshPEM, fetchErr := v.fetchFreshPublicKeyPEM(ctx, actorIRI)
		if fetchErr == nil {
			// Save/Archive the newly resolved key so it is cached
			now := time.Now()
			_ = v.storage.ArchiveKeyHistory(ctx, actorIRI, keyType, freshPEM, now.Add(-24*time.Hour), now.Add(100*365*24*time.Hour))
			v.assertActorIdentityHeader(r, actorIRI)
			return freshPEM, false, nil
		}
		return "", false, fmt.Errorf("failed to fetch remote public key: %w", fetchErr)
	}

	return "", false, fmt.Errorf("remote verification fetch loop not implemented in sandbox or fetcher missing")
}

func (v *FederatedSignatureVerifier) fetchFreshPublicKeyPEM(ctx context.Context, actorIRI string) (string, error) {
	if v.fetcher == nil {
		return "", fmt.Errorf("remote fetcher not configured")
	}

	body, err := v.fetcher.FetchSigned(ctx, actorIRI, "", "", "")
	if err != nil {
		return "", fmt.Errorf("failed to fetch remote actor profile: %w", err)
	}

	// Parse remote actor profile to extract publicKey -> publicKeyPem
	var profile struct {
		PublicKey struct {
			ID           string `json:"id"`
			Owner        string `json:"owner"`
			PublicKeyPem string `json:"publicKeyPem"`
		} `json:"publicKey"`
	}

	if err := json.Unmarshal(body, &profile); err != nil {
		return "", fmt.Errorf("failed to parse remote actor profile: %w", err)
	}

	if profile.PublicKey.PublicKeyPem == "" {
		return "", fmt.Errorf("no publicKeyPem found in remote actor profile")
	}

	return profile.PublicKey.PublicKeyPem, nil
}

// assertActorIdentityHeader notifies downstream middleware of verified identity strings [source: 3].
func (v *FederatedSignatureVerifier) assertActorIdentityHeader(r *http.Request, actorIRI string) {
	if r != nil {
		r.Header.Set("X-Actor-IRI", actorIRI) // Sets identity directly for the middleware
	}
}

// Isolated helper function to decode PEM text directly back to operational RSA object values cleanly
func decodePEMToPublicKey(publicPEM string) (interface{}, error) {
	block, _ := pem.Decode([]byte(publicPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM bytes")
	}

	// 1. Try parsing as a standard PKCS8 Private Key (Handles both our local RSA and Ed25519 row contents)
	if parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		switch key := parsedKey.(type) {
		case *rsa.PrivateKey:
			return &key.PublicKey, nil
		case ed25519.PrivateKey:
			return key.Public(), nil
		default:
			return nil, fmt.Errorf("unsupported PKCS8 private key type")
		}
	}

	// 2. Try parsing as a legacy PKCS1 RSA Private Key structure
	if privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return &privKey.PublicKey, nil
	}

	// 3. Fallback: Parse as a standard PKIX Public Key document
	pubKey, parseErr := x509.ParsePKIXPublicKey(block.Bytes)
	if parseErr == nil {
		return pubKey, nil
	}

	return nil, fmt.Errorf("failed to process incoming verification bytes via all parsers: %w", parseErr)
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

func parseActorIRI(body []byte) string {
	var activity struct {
		Actor interface{} `json:"actor"`
	}
	if err := json.Unmarshal(body, &activity); err != nil {
		return ""
	}
	return parseStringOrID(activity.Actor)
}

func parseStringOrID(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		keys := []string{"id", "@id", "@value"}
		for _, k := range keys {
			if s := parseStringOrID(v[k]); s != "" {
				return s
			}
		}
	case []interface{}:
		for _, item := range v {
			if s := parseStringOrID(item); s != "" {
				return s
			}
		}
	}
	return ""
}

func extractDomain(iri string) string {
	u, err := url.Parse(iri)
	if err != nil {
		return ""
	}
	host := u.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(host)
}

func validateActorDomain(method string, body []byte, keyID string) error {
	if method != http.MethodPost || len(body) == 0 {
		return nil
	}
	actorIRI := parseActorIRI(body)
	if actorIRI == "" {
		return nil
	}
	keyDomain := extractDomain(keyID)
	actorDomain := extractDomain(actorIRI)
	if keyDomain == "" || actorDomain == "" || keyDomain != actorDomain {
		return fmt.Errorf("security violation: signature keyId domain %q does not match actor domain %q", keyDomain, actorDomain)
	}
	return nil
}

func verifySignature(pubKey interface{}, signingString string, signatureBytes []byte) error {
	switch key := pubKey.(type) {
	case *rsa.PublicKey:
		hashed := sha256.Sum256([]byte(signingString))
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], signatureBytes); err != nil {
			return fmt.Errorf("cryptographic validation failed: RSA signature mismatch")
		}
	case ed25519.PublicKey:
		// Ed25519 verifies over the raw signing string text bytes directly, without pre-hashing
		if !ed25519.Verify(key, []byte(signingString), signatureBytes) {
			return fmt.Errorf("cryptographic validation failed: Ed25519 signature mismatch")
		}
	default:
		return fmt.Errorf("unsupported public key type encountered during verification pass")
	}
	return nil
}
