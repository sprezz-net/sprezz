package integrityproof

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"strings"

	"sprezz/internal/pkg/base58"
	"sprezz/internal/pkg/jcs"
)

// ParseEd25519PublicKeyMultibase parses a public key multibase string (e.g. "z6Mkr...")
// into an ed25519.PublicKey.
func ParseEd25519PublicKeyMultibase(multibaseKey string) (ed25519.PublicKey, error) {
	if !strings.HasPrefix(multibaseKey, "z") {
		return nil, fmt.Errorf("invalid multibase public key encoding: must start with 'z'")
	}
	raw, err := base58.Decode(multibaseKey[1:])
	if err != nil {
		return nil, fmt.Errorf("failed to decode base58 public key: %w", err)
	}
	// Ed25519 Multikey multicodec prefix: 0xed, 0x01
	if len(raw) != 34 || raw[0] != 0xed || raw[1] != 0x01 {
		return nil, fmt.Errorf("invalid Ed25519 multicodec public key prefix or length")
	}
	return ed25519.PublicKey(raw[2:]), nil
}

// ParseEd25519PrivateKeyMultibase parses a private key multibase string (e.g. "z3u2e...")
// into an ed25519.PrivateKey.
func ParseEd25519PrivateKeyMultibase(multibaseKey string) (ed25519.PrivateKey, error) {
	if !strings.HasPrefix(multibaseKey, "z") {
		return nil, fmt.Errorf("invalid multibase private key encoding: must start with 'z'")
	}
	raw, err := base58.Decode(multibaseKey[1:])
	if err != nil {
		return nil, fmt.Errorf("failed to decode base58 private key: %w", err)
	}
	// Ed25519 Multikey multicodec prefix: 0x80, 0x26
	if len(raw) != 34 || raw[0] != 0x80 || raw[1] != 0x26 {
		return nil, fmt.Errorf("invalid Ed25519 multicodec private key prefix or length")
	}
	return ed25519.NewKeyFromSeed(raw[2:]), nil
}

// SignDataIntegrityProof signs a document and appends a DataIntegrityProof under the "proof" key.
func SignDataIntegrityProof(docMap map[string]interface{}, privKey ed25519.PrivateKey, keyID string, created string) (map[string]interface{}, error) {
	// 1. Prepare proof configuration
	proofConfig := map[string]interface{}{
		"@context": []interface{}{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/data-integrity/v2",
		},
		"type":               "DataIntegrityProof",
		"cryptosuite":        "eddsa-jcs-2022",
		"verificationMethod": keyID,
		"proofPurpose":       "assertionMethod",
		"created":            created,
	}

	// 2. Prepare document (delete "proof" key)
	docToSign := make(map[string]interface{})
	for k, v := range docMap {
		if k != "proof" {
			docToSign[k] = v
		}
	}

	// 3. Canonicalize document using JCS and compute SHA-256 hash
	docBytes, err := jcs.Format(docToSign)
	if err != nil {
		return nil, fmt.Errorf("failed to canonicalize document: %w", err)
	}
	docHash := sha256.Sum256(docBytes)

	// 4. Canonicalize proof configuration using JCS and compute SHA-256 hash
	proofBytes, err := jcs.Format(proofConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to canonicalize proof configuration: %w", err)
	}
	proofHash := sha256.Sum256(proofBytes)

	// 5. Combine hashes (proofHash followed by docHash)
	sigInput := append(proofHash[:], docHash[:]...)

	// 6. Generate Ed25519 signature
	sigBytes := ed25519.Sign(privKey, sigInput)

	// 7. Base58btc encode signature and prepend "z"
	proofValue := "z" + base58.Encode(sigBytes)

	// 8. Append proof to original document
	signedDoc := make(map[string]interface{})
	for k, v := range docMap {
		signedDoc[k] = v
	}
	signedProof := make(map[string]interface{})
	for k, v := range proofConfig {
		signedProof[k] = v
	}
	signedProof["proofValue"] = proofValue
	signedDoc["proof"] = signedProof

	return signedDoc, nil
}

// VerifyDataIntegrityProof verifies a DataIntegrityProof inside a document.
func VerifyDataIntegrityProof(docMap map[string]interface{}, pubKey ed25519.PublicKey) (bool, error) {
	proofVal, exists := docMap["proof"]
	if !exists {
		return false, fmt.Errorf("missing proof object in document")
	}

	proofMap, ok := proofVal.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("invalid proof structure: expected JSON object")
	}

	cryptosuite, _ := proofMap["cryptosuite"].(string)
	if cryptosuite != "eddsa-jcs-2022" {
		return false, fmt.Errorf("unsupported cryptosuite: %q", cryptosuite)
	}

	proofValue, _ := proofMap["proofValue"].(string)
	if proofValue == "" || !strings.HasPrefix(proofValue, "z") {
		return false, fmt.Errorf("invalid or missing proofValue signature encoding")
	}

	// 1. Prepare proof configuration by removing proofValue
	proofConfig := make(map[string]interface{})
	for k, v := range proofMap {
		if k != "proofValue" {
			proofConfig[k] = v
		}
	}

	// 2. Prepare document (delete "proof" key)
	docToVerify := make(map[string]interface{})
	for k, v := range docMap {
		if k != "proof" {
			docToVerify[k] = v
		}
	}

	// 3. Canonicalize document using JCS and compute SHA-256 hash
	docBytes, err := jcs.Format(docToVerify)
	if err != nil {
		return false, fmt.Errorf("failed to canonicalize document: %w", err)
	}
	docHash := sha256.Sum256(docBytes)

	// 4. Canonicalize proof configuration using JCS and compute SHA-256 hash
	proofBytes, err := jcs.Format(proofConfig)
	if err != nil {
		return false, fmt.Errorf("failed to canonicalize proof configuration: %w", err)
	}
	proofHash := sha256.Sum256(proofBytes)

	// 5. Combine hashes (proofHash followed by docHash)
	sigInput := append(proofHash[:], docHash[:]...)

	// 6. Decode Base58btc signature
	sigBytes, err := base58.Decode(proofValue[1:])
	if err != nil {
		return false, fmt.Errorf("failed to decode proofValue signature: %w", err)
	}

	// 7. Verify signature
	if !ed25519.Verify(pubKey, sigInput, sigBytes) {
		return false, fmt.Errorf("cryptographic verification failed: Ed25519 signature mismatch")
	}

	return true, nil
}
