package cryptoutil_test

import (
	"crypto/sha256"
	"sprezz/internal/pkg/cryptoutil"
	"testing"
)

func TestToDigestMultibase(t *testing.T) {
	data := []byte("hello world")
	hash := sha256.Sum256(data)
	multibaseStr := cryptoutil.ToDigestMultibase(hash[:])
	if multibaseStr == "" {
		t.Fatal("expected non-empty multibase digest string")
	}
	if multibaseStr[0] != 'z' {
		t.Errorf("expected multibase string to start with 'z', got %q", multibaseStr)
	}
	// A valid multibase base58btc SHA-256 multihash always starts with "zQm"
	if multibaseStr[:3] != "zQm" {
		t.Errorf("expected multibase string to start with 'zQm', got %q", multibaseStr[:3])
	}
}
