package cryptoutil_test

import (
	"encoding/json"
	"testing"

	"sprezz/internal/pkg/cryptoutil"
)

const (
	testDocumentJSON = `{
		"@context": [
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/data-integrity/v2"
		],
		"id": "https://server.example/activities/1",
		"type": "Create",
		"actor": "https://server.example/users/alice",
		"object": {
			"id": "https://server.example/objects/1",
			"type": "Note",
			"attributedTo": "https://server.example/users/alice",
			"content": "Hello world",
			"location": {
				"type": "Place",
				"longitude": -71.184902,
				"latitude": 25.273962
			}
		}
	}`
	testSecretKeyMultibase = "z3u2en7t5LR2WtQH5PfFqMqwVHBeXouLzo6haApm8XHqvjxq"
	testCreatedTime        = "2023-02-24T23:36:38Z"
	testVerificationMethod = "https://server.example/users/alice#ed25519-key"
)

func TestIntegrityProof_CanonicalizeDoc(t *testing.T) {
	var docMap map[string]interface{}
	if err := json.Unmarshal([]byte(testDocumentJSON), &docMap); err != nil {
		t.Fatalf("Failed to parse document JSON: %v", err)
	}

	docToSign := make(map[string]interface{})
	for k, v := range docMap {
		if k != "proof" {
			docToSign[k] = v
		}
	}

	docBytes, err := cryptoutil.FormatJCS(docToSign)
	if err != nil {
		t.Fatalf("Failed JCS formatting: %v", err)
	}

	expectedDoc := `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/data-integrity/v2"],"actor":"https://server.example/users/alice","id":"https://server.example/activities/1","object":{"attributedTo":"https://server.example/users/alice","content":"Hello world","id":"https://server.example/objects/1","location":{"latitude":25.273962,"longitude":-71.184902,"type":"Place"},"type":"Note"},"type":"Create"}`
	if string(docBytes) != expectedDoc {
		t.Errorf("Canonicalized document mismatch.\nGot : %s\nWant: %s", string(docBytes), expectedDoc)
	}
}

func TestIntegrityProof_CanonicalizeProof(t *testing.T) {
	proofConfig := map[string]interface{}{
		"@context": []interface{}{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/data-integrity/v2",
		},
		"type":               "DataIntegrityProof",
		"cryptosuite":        "eddsa-jcs-2022",
		"verificationMethod": testVerificationMethod,
		"proofPurpose":       "assertionMethod",
		"created":            testCreatedTime,
	}

	proofBytes, err := cryptoutil.FormatJCS(proofConfig)
	if err != nil {
		t.Fatalf("Failed JCS proof formatting: %v", err)
	}

	expectedProof := `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/data-integrity/v2"],"created":"2023-02-24T23:36:38Z","cryptosuite":"eddsa-jcs-2022","proofPurpose":"assertionMethod","type":"DataIntegrityProof","verificationMethod":"https://server.example/users/alice#ed25519-key"}`
	if string(proofBytes) != expectedProof {
		t.Errorf("Canonicalized proof mismatch.\nGot : %s\nWant: %s", string(proofBytes), expectedProof)
	}
}

func TestIntegrityProof_SigningAndVerification(t *testing.T) {
	var docMap map[string]interface{}
	if err := json.Unmarshal([]byte(testDocumentJSON), &docMap); err != nil {
		t.Fatalf("Failed to parse document JSON: %v", err)
	}

	privKey, err := cryptoutil.ParseEd25519PrivateKeyMultibase(testSecretKeyMultibase)
	if err != nil {
		t.Fatalf("Failed to parse private key multibase: %v", err)
	}

	signedDoc, err := cryptoutil.SignDataIntegrityProof(docMap, privKey, testVerificationMethod, testCreatedTime)
	if err != nil {
		t.Fatalf("Failed to sign document: %v", err)
	}

	proofObj, exists := signedDoc["proof"]
	if !exists {
		t.Fatalf("Signed document missing proof field")
	}

	proofMap, ok := proofObj.(map[string]interface{})
	if !ok {
		t.Fatalf("Proof field is not a map")
	}

	expectedProofValue := "z42ffGu6AUKPCFcFPiabmUvnGLPJzC7e4DGWC52NUasSSH37UMa9c58tdgVszUcZfytxa4fQ5TYHaJENCxUDe9SdL"
	proofValue, _ := proofMap["proofValue"].(string)
	if proofValue != expectedProofValue {
		t.Errorf("Generated proofValue mismatch.\nGot : %s\nWant: %s", proofValue, expectedProofValue)
	}

	actorPublicKeyMultibase := "z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2"
	pubKey, err := cryptoutil.ParseEd25519PublicKeyMultibase(actorPublicKeyMultibase)
	if err != nil {
		t.Fatalf("Failed to parse public key multibase: %v", err)
	}

	valid, err := cryptoutil.VerifyDataIntegrityProof(signedDoc, pubKey)
	if err != nil {
		t.Fatalf("Failed to verify document: %v", err)
	}
	if !valid {
		t.Errorf("Verify returned false, expected true")
	}
}

func TestIntegrityProof_InvalidSignatures(t *testing.T) {
	tamperedJSON := `{
		"@context": [
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/data-integrity/v2"
		],
		"id": "https://server.example/activities/1",
		"type": "Create",
		"actor": "https://server.example/users/alice",
		"object": {
			"id": "https://server.example/objects/1",
			"type": "Note",
			"attributedTo": "https://server.example/users/alice",
			"content": "Hello world (TAMPERED)"
		},
		"proof": {
			"@context": [
				"https://www.w3.org/ns/activitystreams",
				"https://w3id.org/security/data-integrity/v2"
			],
			"type": "DataIntegrityProof",
			"cryptosuite": "eddsa-jcs-2022",
			"verificationMethod": "https://server.example/users/alice#ed25519-key",
			"proofPurpose": "assertionMethod",
			"proofValue": "z42ffGu6AUKPCFcFPiabmUvnGLPJzC7e4DGWC52NUasSSH37UMa9c58tdgVszUcZfytxa4fQ5TYHaJENCxUDe9SdL",
			"created": "2023-02-24T23:36:38Z"
		}
	}`

	var docMap map[string]interface{}
	if err := json.Unmarshal([]byte(tamperedJSON), &docMap); err != nil {
		t.Fatalf("Failed to parse tampered JSON: %v", err)
	}

	actorPublicKeyMultibase := "z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2"
	pubKey, _ := cryptoutil.ParseEd25519PublicKeyMultibase(actorPublicKeyMultibase)

	valid, err := cryptoutil.VerifyDataIntegrityProof(docMap, pubKey)
	if err == nil {
		t.Errorf("Expected verification error for tampered document, got nil")
	}
	if valid {
		t.Errorf("Expected verification to fail, but it returned true")
	}
}
