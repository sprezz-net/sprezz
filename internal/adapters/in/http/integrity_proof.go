package http

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"sprezz/internal/pkg/integrityproof"
)

// CheckAndVerifyIntegrityProof checks for the presence of a FEP-8b32 Object Integrity Proof.
// It returns (true, nil) if verified successfully, (true, err) if proof is present but verification fails,
// and (false, nil) if no integrity proof is present in the request body.
func (v *FederatedSignatureVerifier) CheckAndVerifyIntegrityProof(r *http.Request, body []byte) (bool, error) {
	if len(body) == 0 {
		return false, nil
	}

	var docMap map[string]interface{}
	if err := json.Unmarshal(body, &docMap); err != nil {
		return false, nil
	}

	proofVal, exists := docMap["proof"]
	if !exists {
		return false, nil
	}

	proofMap, ok := proofVal.(map[string]interface{})
	if !ok {
		return false, nil
	}

	cryptosuite, _ := proofMap["cryptosuite"].(string)
	verificationMethod, _ := proofMap["verificationMethod"].(string)
	if cryptosuite != "eddsa-jcs-2022" || verificationMethod == "" {
		return false, nil
	}

	ctx := r.Context()
	publicPEM, err := v.resolvePublicKeyPEM(ctx, r, verificationMethod, r.Header.Get("Date"))
	if err != nil {
		return true, fmt.Errorf("resolve integrity verification key: %w", err)
	}

	pubKey, err := decodePEMToPublicKey(publicPEM)
	if err != nil {
		return true, fmt.Errorf("decode integrity verification key: %w", err)
	}

	ed25519PubKey, ok := pubKey.(ed25519.PublicKey)
	if !ok {
		return true, fmt.Errorf("verification key is not an Ed25519 public key")
	}

	valid, err := integrityproof.VerifyDataIntegrityProof(docMap, ed25519PubKey)
	if err != nil {
		return true, err
	}
	if !valid {
		return true, fmt.Errorf("integrity proof cryptographic mismatch")
	}

	actorIRI := strings.Split(verificationMethod, "#")[0]
	v.assertActorIdentityHeader(r, actorIRI)
	return true, nil
}
