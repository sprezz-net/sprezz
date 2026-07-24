package http

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type PublicKeyResolver interface {
	ResolvePublicKey(keyID string) (*rsa.PublicKey, error)
}

type SignatureVerifier struct {
	resolver PublicKeyResolver
	now      func() time.Time
	maxAge   time.Duration
}

func NewSignatureVerifier(resolver PublicKeyResolver) *SignatureVerifier {
	return &SignatureVerifier{resolver: resolver, now: time.Now, maxAge: 5 * time.Minute}
}

func (v *SignatureVerifier) Verify(r *http.Request, body []byte) error {
	if v == nil || v.resolver == nil {
		return fmt.Errorf("signature verifier is not configured")
	}
	digest := r.Header.Get("Digest")
	if digest == "" {
		return fmt.Errorf("missing digest header")
	}
	if err := verifyDigest(digest, body); err != nil {
		return err
	}
	keyID, headers, signature, err := parseSignature(r.Header.Get("Signature"))
	if err != nil {
		return err
	}
	date := r.Header.Get("Date")
	parsedDate, err := http.ParseTime(date)
	if err != nil || v.now().Sub(parsedDate) > v.maxAge || parsedDate.Sub(v.now()) > v.maxAge {
		return fmt.Errorf("stale or invalid date header")
	}
	for _, name := range headers {
		if name != "(request-target)" && name != "host" && name != "date" && name != "digest" {
			return fmt.Errorf("unsigned request component %q is not allowed", name)
		}
	}
	canonical, err := signingString(r, headers)
	if err != nil {
		return err
	}
	key, err := v.resolver.ResolvePublicKey(keyID)
	if err != nil {
		return fmt.Errorf("resolve signature key: %w", err)
	}
	signatureBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	hash := sha256.Sum256([]byte(canonical))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signatureBytes); err != nil {
		return fmt.Errorf("invalid request signature: %w", err)
	}
	return nil
}

func verifyDigest(value string, body []byte) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "SHA-256") {
		return fmt.Errorf("unsupported or invalid digest header")
	}
	hash := sha256.Sum256(body)
	if !strings.EqualFold(parts[1], base64.StdEncoding.EncodeToString(hash[:])) {
		return fmt.Errorf("request digest does not match body")
	}
	return nil
}

func parseSignature(value string) (string, []string, string, error) {
	if value == "" {
		return "", nil, "", fmt.Errorf("missing signature header")
	}
	fields := make(map[string]string)
	for _, part := range strings.Split(value, ",") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 {
			return "", nil, "", fmt.Errorf("invalid signature parameter")
		}
		fields[pair[0]] = strings.Trim(strings.TrimSpace(pair[1]), "\"")
	}
	keyID, headers, signature := fields["keyId"], strings.Fields(fields["headers"]), fields["signature"]
	if keyID == "" || len(headers) == 0 || signature == "" {
		return "", nil, "", fmt.Errorf("signature is missing required fields")
	}
	return keyID, headers, signature, nil
}

func signingString(r *http.Request, headers []string) (string, error) {
	values := make([]string, 0, len(headers))
	for _, name := range headers {
		var value string
		switch name {
		case "(request-target)":
			value = strings.ToLower(r.Method) + " " + r.URL.RequestURI()
		case "host":
			value = RequestHost(r)
		case "date":
			value = r.Header.Get("Date")
		case "digest":
			value = r.Header.Get("Digest")
		default:
			return "", fmt.Errorf("unsupported signature header %q", name)
		}
		values = append(values, name+": "+value)
	}
	return strings.Join(values, "\n"), nil
}

type HTTPPublicKeyResolver struct {
	client *http.Client
}

func NewHTTPPublicKeyResolver(client *http.Client) *HTTPPublicKeyResolver {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &HTTPPublicKeyResolver{client: client}
}

func (r *HTTPPublicKeyResolver) ResolvePublicKey(keyID string) (*rsa.PublicKey, error) {
	u, err := url.Parse(keyID)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, fmt.Errorf("key ID must be an HTTPS URL")
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("resolve key host")
	}
	for _, ip := range ips {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return nil, fmt.Errorf("key host resolves to a private address")
		}
	}
	resp, err := r.client.Get(keyID)
	if err != nil {
		return nil, err
	}
	// FIXED: Handled the response close error value tracking through an explicit discard closure to pass strict errcheck criteria
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("key endpoint returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var document struct {
		PublicKey struct {
			PEM string `json:"publicKeyPem"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(document.PublicKey.PEM))
	if block == nil {
		return nil, fmt.Errorf("public key PEM missing")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return key, nil
}
