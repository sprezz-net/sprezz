package service_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"sprezz/internal/domain/service"
)

func TestFEP8c13_AuthorProof(t *testing.T) {
	// 1. Generate keys
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	docMap := map[string]interface{}{
		"@context": []interface{}{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/data-integrity/v1",
			"https://w3id.org/fep/8c13",
		},
		"id":    "https://local.example/activities/1",
		"type":  "Create",
		"actor": "https://local.example/users/alice",
		"to":    []interface{}{"https://www.w3.org/ns/activitystreams#Public"},
		"cc":    []interface{}{"https://local.example/users/bob"},
		"object": map[string]interface{}{
			"id":      "https://local.example/objects/1",
			"type":    "Note",
			"content": "Secret content",
			"to":      []interface{}{"https://www.w3.org/ns/activitystreams#Public"},
			"cc":      []interface{}{"https://local.example/users/bob"},
		},
	}

	keyID := "https://local.example/users/alice#ed25519-key"
	created := "2026-01-08T12:00:00Z"

	// 2. Sign Author Proof
	signedDoc, err := service.SignAuthorProof(docMap, priv, keyID, created)
	if err != nil {
		t.Fatalf("failed to sign author proof: %v", err)
	}

	// Ensure original to/cc are still in the signed doc
	if _, exists := signedDoc["to"]; !exists {
		t.Error("expected 'to' to be retained in the signed document")
	}

	// 3. Verify Author Proof
	ok, err := service.VerifyAuthorProof(signedDoc, pub)
	if err != nil {
		t.Fatalf("failed to verify author proof: %v", err)
	}
	if !ok {
		t.Error("author proof cryptographic verification failed")
	}

	// 4. Test that modifying addressing (rewiring to/cc) does NOT break the author proof
	signedDoc["to"] = []interface{}{"https://local.example/users/carol"}
	objMap := signedDoc["object"].(map[string]interface{})
	objMap["to"] = []interface{}{"https://local.example/users/carol"}

	ok, err = service.VerifyAuthorProof(signedDoc, pub)
	if err != nil {
		t.Fatalf("failed to verify author proof after rewiring: %v", err)
	}
	if !ok {
		t.Error("author proof broke after rewiring addressing fields, which violates FEP-8c13 requirements!")
	}

	// 5. Test that modifying actual content DOES break the author proof
	objMap["content"] = "Tampered content"
	ok, err = service.VerifyAuthorProof(signedDoc, pub)
	if err == nil && ok {
		t.Error("author proof verification should have failed after content tampering")
	}
}

func TestFEP8c13_ForwardingProof(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Prepare standard Author Proof signed activity
	docMap := map[string]interface{}{
		"@context": []interface{}{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/data-integrity/v1",
			"https://w3id.org/fep/8c13",
		},
		"id":             "https://bob.example/activities/98765",
		"type":           "Create",
		"actor":          "https://bob.example/u/bob",
		"contextHistory": "https://alice.example/history/12345",
		"to":             []interface{}{"https://alice.example/u/alice/followers"},
		"cc":             []interface{}{"https://alice.example/u/alice"},
		"object": map[string]interface{}{
			"id":             "https://bob.example/posts/98765",
			"type":           "Note",
			"contextHistory": "https://alice.example/history/12345",
			"content":        "Hi Alice",
		},
	}

	_, authorPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	signedDoc, err := service.SignAuthorProof(docMap, authorPriv, "https://bob.example/u/bob#key", "2026-01-08T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}

	// Now Context Authority signs the Forwarding Proof
	authorityKeyID := "https://alice.example/actor#key"
	forwardedDoc, err := service.SignForwardingProof(signedDoc, priv, authorityKeyID, "2026-01-08T12:01:00Z")
	if err != nil {
		t.Fatalf("failed to sign forwarding proof: %v", err)
	}

	// Verify Forwarding Proof
	ok, err := service.VerifyForwardingProof(forwardedDoc, pub)
	if err != nil {
		t.Fatalf("failed to verify forwarding proof: %v", err)
	}
	if !ok {
		t.Error("forwarding proof verification failed")
	}

	// Test that modifying addressing (rewiring to/cc) DOES break the forwarding proof
	forwardedDoc["to"] = []interface{}{"https://alice.example/u/alice/followers", "https://tampered-recipient.com"}
	ok, err = service.VerifyForwardingProof(forwardedDoc, pub)
	if err == nil && ok {
		t.Error("forwarding proof should fail verification if addressing fields are tampered with")
	}
}

func TestFEP8c13_GetEffectiveContextHistoryIRI(t *testing.T) {
	tests := []struct {
		name    string
		doc     map[string]interface{}
		want    string
		wantErr bool
	}{
		{
			name: "Only activity has contextHistory",
			doc: map[string]interface{}{
				"contextHistory": "https://example.com/context",
			},
			want: "https://example.com/context",
		},
		{
			name: "Only embedded object has contextHistory",
			doc: map[string]interface{}{
				"object": map[string]interface{}{
					"contextHistory": "https://example.com/context",
				},
			},
			want: "https://example.com/context",
		},
		{
			name: "Both are present and equal",
			doc: map[string]interface{}{
				"contextHistory": "https://example.com/context",
				"object": map[string]interface{}{
					"contextHistory": "https://example.com/context",
				},
			},
			want: "https://example.com/context",
		},
		{
			name: "Both are present and differ",
			doc: map[string]interface{}{
				"contextHistory": "https://example.com/context-a",
				"object": map[string]interface{}{
					"contextHistory": "https://example.com/context-b",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.GetEffectiveContextHistoryIRI(tt.doc)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetEffectiveContextHistoryIRI() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetEffectiveContextHistoryIRI() = %v, want %v", got, tt.want)
			}
		})
	}
}
