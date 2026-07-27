package model

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

type GeneratedDualKeys struct {
	RSAPrivatePEM     string
	Ed25519PrivatePEM string
}

// MintNewKeyPair centralizes the 2048-bit RSA and Ed25519 key-generation workflow [source: 5].
func MintNewKeyPair() (*GeneratedDualKeys, error) {
	privKeyRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate secure RSA matrix: %w", err)
	}
	rsaBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privKeyRSA)}
	rsaPEM := string(pem.EncodeToMemory(rsaBlock))

	_, privKeyEd, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate secure Ed25519 matrix: %w", err)
	}
	edBytes, err := x509.MarshalPKCS8PrivateKey(privKeyEd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Ed25519 keys: %w", err)
	}
	edBlock := &pem.Block{Type: "PRIVATE KEY", Bytes: edBytes}
	edPEM := string(pem.EncodeToMemory(edBlock))

	return &GeneratedDualKeys{
		RSAPrivatePEM:     rsaPEM,
		Ed25519PrivatePEM: edPEM,
	}, nil
}

// ExtractRSAPublicKey derives a public key block from an RSA private PEM string [source: 5].
func ExtractRSAPublicKey(privateKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to decode RSA private key PEM block structure")
	}

	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		parsedKey, err8 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err8 != nil {
			return "", fmt.Errorf("failed parsing private key via PKCS1 (%v) and PKCS8 (%v)", err, err8)
		}
		rsaKey, ok := parsedKey.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("decoded private key block is not a valid RSA type")
		}
		privKey = rsaKey
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal PKIX RSA public key payload bytes: %w", err)
	}

	pubBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes}
	return string(pem.EncodeToMemory(pubBlock)), nil
}

// ExtractEd25519PublicKey derives a public key block from an Ed25519 private PEM string [source: 5].
func ExtractEd25519PublicKey(privateKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to decode Ed25519 private key PEM block structure")
	}

	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed parsing Ed25519 private key via PKCS8 rules: %w", err)
	}

	edPrivKey, ok := parsedKey.(ed25519.PrivateKey)
	if !ok {
		return "", fmt.Errorf("decoded private key block is not a valid Ed25519 type")
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(edPrivKey.Public())
	if err != nil {
		return "", fmt.Errorf("failed to marshal PKIX Ed25519 public key payload bytes: %w", err)
	}

	pubBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes}
	return string(pem.EncodeToMemory(pubBlock)), nil
}
