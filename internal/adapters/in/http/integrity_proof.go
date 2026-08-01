package http

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"sprezz/internal/domain/service"
	"sprezz/internal/pkg/cryptoutil"
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

	ctx := r.Context()

	// Try FEP-8c13 Author Proof verification first
	if authorProofVal, authorExists := docMap["authorProof"]; authorExists {
		verificationMethod, err := v.verifyAuthorProofPath(ctx, r, docMap, authorProofVal)
		if err != nil {
			return true, err
		}
		actorIRI := strings.Split(verificationMethod, "#")[0]
		v.assertActorIdentityHeader(r, actorIRI)
		return true, nil
	}

	// Try FEP-8b32 Object Integrity Proof second
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

	ed25519PubKey, err := v.resolveEd25519PublicKey(ctx, r, verificationMethod)
	if err != nil {
		return true, fmt.Errorf("resolve integrity verification key: %w", err)
	}

	valid, err := cryptoutil.VerifyDataIntegrityProof(docMap, ed25519PubKey)
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

func (v *FederatedSignatureVerifier) resolveEd25519PublicKey(ctx context.Context, r *http.Request, verificationMethod string) (ed25519.PublicKey, error) {
	publicPEM, _, err := v.resolvePublicKeyPEM(ctx, r, verificationMethod, r.Header.Get("Date"))
	if err != nil {
		return nil, fmt.Errorf("resolve key: %w", err)
	}

	pubKey, err := decodePEMToPublicKey(publicPEM)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}

	ed25519PubKey, ok := pubKey.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not an Ed25519 public key")
	}

	return ed25519PubKey, nil
}

func (v *FederatedSignatureVerifier) verifyForwardingProof(ctx context.Context, r *http.Request, docMap map[string]interface{}, forwardingProofVal interface{}) error {
	fwdProofMap, ok := forwardingProofVal.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid forwardingProof structure")
	}
	fwdVerificationMethod, _ := fwdProofMap["verificationMethod"].(string)
	if fwdVerificationMethod == "" {
		return fmt.Errorf("missing verificationMethod in forwardingProof")
	}

	fwdEd25519PubKey, err := v.resolveEd25519PublicKey(ctx, r, fwdVerificationMethod)
	if err != nil {
		return fmt.Errorf("resolve forwarding integrity key: %w", err)
	}

	fwdValid, err := service.VerifyForwardingProof(docMap, fwdEd25519PubKey)
	if err != nil {
		return err
	}
	if !fwdValid {
		return fmt.Errorf("forwarding proof cryptographic mismatch")
	}

	return nil
}

func (v *FederatedSignatureVerifier) verifyAuthorProofPath(ctx context.Context, r *http.Request, docMap map[string]interface{}, authorProofVal interface{}) (string, error) {
	authorProofMap, ok := authorProofVal.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid authorProof structure")
	}
	cryptosuite, _ := authorProofMap["cryptosuite"].(string)
	verificationMethod, _ := authorProofMap["verificationMethod"].(string)
	if cryptosuite != "eddsa-jcs-2022" || verificationMethod == "" {
		return "", fmt.Errorf("unsupported cryptosuite or verificationMethod in authorProof")
	}

	ed25519PubKey, err := v.resolveEd25519PublicKey(ctx, r, verificationMethod)
	if err != nil {
		return "", fmt.Errorf("resolve author integrity key: %w", err)
	}

	valid, err := service.VerifyAuthorProof(docMap, ed25519PubKey)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", fmt.Errorf("author proof cryptographic mismatch")
	}

	if forwardingProofVal, fwdExists := docMap["forwardingProof"]; fwdExists {
		if err := v.verifyForwardingProof(ctx, r, docMap, forwardingProofVal); err != nil {
			return "", err
		}
	}

	return verificationMethod, nil
}
