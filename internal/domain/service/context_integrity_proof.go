package service

import (
	"crypto/ed25519"
	"fmt"
	"sort"

	"sprezz/internal/pkg/cryptoutil"
)

// excludeAuthorProofFields recursively removes "to", "cc", "forwardingProof", and legacy "signature" fields
func excludeAuthorProofFields(val interface{}) interface{} {
	switch v := val.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{})
		for k, valItem := range v {
			if k == "to" || k == "cc" || k == "forwardingProof" || k == "signature" {
				continue
			}
			cloned[k] = excludeAuthorProofFields(valItem)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(v))
		for i, item := range v {
			cloned[i] = excludeAuthorProofFields(item)
		}
		return cloned
	default:
		return val
	}
}

// GetEffectiveContextHistoryIRI extracts the Effective Context History IRI per FEP-8c13 rules.
func GetEffectiveContextHistoryIRI(docMap map[string]interface{}) (string, error) {
	var activityIRI string
	if actIRIVal, exists := docMap["contextHistory"]; exists {
		activityIRI, _ = actIRIVal.(string)
	}

	var objIRI string
	if objVal, exists := docMap["object"]; exists {
		if objMap, ok := objVal.(map[string]interface{}); ok {
			if objIRIVal, exists := objMap["contextHistory"]; exists {
				objIRI, _ = objIRIVal.(string)
			}
		}
	}

	if activityIRI != "" && objIRI != "" {
		if activityIRI != objIRI {
			return "", fmt.Errorf("contextHistory mismatch: activity contextHistory %q differs from object contextHistory %q", activityIRI, objIRI)
		}
		return activityIRI, nil
	}

	if activityIRI != "" {
		return activityIRI, nil
	}

	return objIRI, nil
}

// normalizeIRIList normalizes the "to" and "cc" string slices per FEP-8c13.
func normalizeIRIList(val interface{}) ([]string, error) {
	if val == nil {
		return []string{}, nil
	}

	var rawStrs []string
	switch v := val.(type) {
	case []interface{}:
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("array element must be a string")
			}
			rawStrs = append(rawStrs, str)
		}
	case []string:
		rawStrs = v
	default:
		return nil, fmt.Errorf("must be an array of strings")
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, s := range rawStrs {
		if !seen[s] {
			seen[s] = true
			unique = append(unique, s)
		}
	}

	// Sort lexicographically
	sort.Strings(unique)
	return unique, nil
}

// VerifyAuthorProof verifies the FEP-8c13 Author Proof inside an ActivityPub document.
func VerifyAuthorProof(docMap map[string]interface{}, pubKey ed25519.PublicKey) (bool, error) {
	authorProofVal, exists := docMap["authorProof"]
	if !exists {
		return false, fmt.Errorf("missing authorProof in document")
	}

	authorProof, ok := authorProofVal.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("invalid authorProof structure: expected JSON object")
	}

	// Transform the document using field exclusions
	docToVerify := excludeAuthorProofFields(docMap).(map[string]interface{})
	delete(docToVerify, "authorProof")
	docToVerify["proof"] = authorProof

	return cryptoutil.VerifyDataIntegrityProof(docToVerify, pubKey)
}

// SignAuthorProof generates and attaches an Author Proof to the given document map.
func SignAuthorProof(docMap map[string]interface{}, privKey ed25519.PrivateKey, keyID string, created string) (map[string]interface{}, error) {
	docToSign := excludeAuthorProofFields(docMap).(map[string]interface{})
	delete(docToSign, "authorProof")

	signedDoc, err := cryptoutil.SignDataIntegrityProof(docToSign, privKey, keyID, created)
	if err != nil {
		return nil, fmt.Errorf("sign author proof: %w", err)
	}

	proofObj, exists := signedDoc["proof"]
	if !exists {
		return nil, fmt.Errorf("signing failed to attach standard proof object")
	}

	// Clone original docMap and attach "authorProof"
	cloned := make(map[string]interface{})
	for k, v := range docMap {
		cloned[k] = v
	}
	cloned["authorProof"] = proofObj
	return cloned, nil
}

// VerifyForwardingProof verifies a FEP-8c13 Forwarding Proof on a forwarded activity.
func VerifyForwardingProof(docMap map[string]interface{}, pubKey ed25519.PublicKey) (bool, error) {
	forwardingProofVal, exists := docMap["forwardingProof"]
	if !exists {
		return false, fmt.Errorf("missing forwardingProof in document")
	}

	forwardingProof, ok := forwardingProofVal.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("invalid forwardingProof structure: expected JSON object")
	}

	payload, err := BuildForwardingProofPayload(docMap)
	if err != nil {
		return false, fmt.Errorf("failed to build forwarding proof payload: %w", err)
	}

	payload["proof"] = forwardingProof

	return cryptoutil.VerifyDataIntegrityProof(payload, pubKey)
}

// SignForwardingProof signs a forwarded activity's addressing and attaches a Forwarding Proof.
func SignForwardingProof(docMap map[string]interface{}, privKey ed25519.PrivateKey, keyID string, created string) (map[string]interface{}, error) {
	payload, err := BuildForwardingProofPayload(docMap)
	if err != nil {
		return nil, fmt.Errorf("failed to build forwarding proof payload: %w", err)
	}

	signedPayload, err := cryptoutil.SignDataIntegrityProof(payload, privKey, keyID, created)
	if err != nil {
		return nil, fmt.Errorf("sign forwarding proof: %w", err)
	}

	proofObj, exists := signedPayload["proof"]
	if !exists {
		return nil, fmt.Errorf("signing failed to attach forwarding proof object")
	}

	// Clone original docMap and attach "forwardingProof"
	cloned := make(map[string]interface{})
	for k, v := range docMap {
		cloned[k] = v
	}
	cloned["forwardingProof"] = proofObj
	return cloned, nil
}

// BuildForwardingProofPayload extracts and normalizes the 8 keys for a Forwarding Proof per FEP-8c13.
func BuildForwardingProofPayload(docMap map[string]interface{}) (map[string]interface{}, error) {
	id, _ := docMap["id"].(string)
	actType, _ := docMap["type"].(string)
	actor, _ := docMap["actor"].(string)

	contextHistory, err := GetEffectiveContextHistoryIRI(docMap)
	if err != nil {
		return nil, err
	}
	if contextHistory == "" {
		return nil, fmt.Errorf("missing or invalid contextHistory")
	}

	var object string
	if objVal, exists := docMap["object"]; exists {
		switch o := objVal.(type) {
		case string:
			object = o
		case map[string]interface{}:
			object, _ = o["id"].(string)
		}
	}
	if object == "" {
		return nil, fmt.Errorf("missing activity object ID")
	}

	toNorm, err := normalizeIRIList(docMap["to"])
	if err != nil {
		return nil, fmt.Errorf("normalize to: %w", err)
	}

	ccNorm, err := normalizeIRIList(docMap["cc"])
	if err != nil {
		return nil, fmt.Errorf("normalize cc: %w", err)
	}

	authorProofVal, exists := docMap["authorProof"]
	if !exists {
		return nil, fmt.Errorf("missing authorProof needed for forwarding proof")
	}
	authorProof, ok := authorProofVal.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid authorProof structure")
	}
	authorProofValue, _ := authorProof["proofValue"].(string)
	if authorProofValue == "" {
		return nil, fmt.Errorf("missing authorProof proofValue")
	}

	return map[string]interface{}{
		"id":               id,
		"type":             actType,
		"actor":            actor,
		"contextHistory":   contextHistory,
		"object":           object,
		"to":               toNorm,
		"cc":               ccNorm,
		"authorProofValue": authorProofValue,
	}, nil
}
