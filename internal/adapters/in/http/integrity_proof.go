package http

import (
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

	// Try FEP-8c13 Author Proof verification first
	if authorProofVal, authorExists := docMap["authorProof"]; authorExists {
		authorProofMap, ok := authorProofVal.(map[string]interface{})
		if !ok {
			return true, fmt.Errorf("invalid authorProof structure")
		}
		cryptosuite, _ := authorProofMap["cryptosuite"].(string)
		verificationMethod, _ := authorProofMap["verificationMethod"].(string)
		if cryptosuite != "eddsa-jcs-2022" || verificationMethod == "" {
			return true, fmt.Errorf("unsupported cryptosuite or verificationMethod in authorProof")
		}

		ctx := r.Context()
		publicPEM, err := v.resolvePublicKeyPEM(ctx, r, verificationMethod, r.Header.Get("Date"))
		if err != nil {
			return true, fmt.Errorf("resolve author integrity key: %w", err)
		}

		pubKey, err := decodePEMToPublicKey(publicPEM)
		if err != nil {
			return true, fmt.Errorf("decode author integrity key: %w", err)
		}

		ed25519PubKey, ok := pubKey.(ed25519.PublicKey)
		if !ok {
			return true, fmt.Errorf("author verification key is not an Ed25519 public key")
		}

		valid, err := service.VerifyAuthorProof(docMap, ed25519PubKey)
		if err != nil {
			return true, err
		}
		if !valid {
			return true, fmt.Errorf("author proof cryptographic mismatch")
		}

		// Also check forwardingProof if present
		if forwardingProofVal, fwdExists := docMap["forwardingProof"]; fwdExists {
			fwdProofMap, ok := forwardingProofVal.(map[string]interface{})
			if !ok {
				return true, fmt.Errorf("invalid forwardingProof structure")
			}
			fwdVerificationMethod, _ := fwdProofMap["verificationMethod"].(string)
			if fwdVerificationMethod == "" {
				return true, fmt.Errorf("missing verificationMethod in forwardingProof")
			}

			fwdPublicPEM, err := v.resolvePublicKeyPEM(ctx, r, fwdVerificationMethod, r.Header.Get("Date"))
			if err != nil {
				return true, fmt.Errorf("resolve forwarding integrity key: %w", err)
			}

			fwdPubKey, err := decodePEMToPublicKey(fwdPublicPEM)
			if err != nil {
				return true, fmt.Errorf("decode forwarding integrity key: %w", err)
			}

			fwdEd25519PubKey, ok := fwdPubKey.(ed25519.PublicKey)
			if !ok {
				return true, fmt.Errorf("forwarding verification key is not an Ed25519 public key")
			}

			fwdValid, err := service.VerifyForwardingProof(docMap, fwdEd25519PubKey)
			if err != nil {
				return true, err
			}
			if !fwdValid {
				return true, fmt.Errorf("forwarding proof cryptographic mismatch")
			}
		}

		actorIRI := strings.Split(verificationMethod, "#")[0]
		v.assertActorIdentityHeader(r, actorIRI)
		return true, nil
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
