package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"

	"sprezz/internal/domain/ports"

	"github.com/google/uuid"
)

type BootstrapService struct {
	storagePort ports.StoragePort
}

func NewBootstrapService(sp ports.StoragePort) *BootstrapService {
	return &BootstrapService{storagePort: sp}
}

// BootstrapTenantsAndServerActors checks and maps configured domains cleanly.
func (s *BootstrapService) BootstrapTenantsAndServerActors(ctx context.Context, configuredDomains []string) error {
	for _, domain := range configuredDomains {
		tenantID, err := s.storagePort.GetOrCreateTenantByDomain(ctx, domain)
		if err != nil {
			return fmt.Errorf("failed to reconcile tenant for domain %s: %w", domain, err)
		}

		exists, err := s.storagePort.HasActorCredential(ctx, tenantID, "server")
		if err != nil {
			return fmt.Errorf("failed to verify server actor status for %s: %w", domain, err)
		}

		if !exists {
			if err := s.provisionServerActor(ctx, domain, tenantID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *BootstrapService) provisionServerActor(ctx context.Context, domain string, tenantID int32) error {
	log.Printf("[Bootstrap] Initiating dual-key cryptographic provisioning for domain: %s", domain)

	actorUUID, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("failed to generate random UUIDv4: %w", err)
	}

	// 1. Generate Legacy Perimeter Edge Key Pair: RSA 2048-bit
	privKeyRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate secure RSA keys: %w", err)
	}
	rsaBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privKeyRSA)}
	rsaPEM := string(pem.EncodeToMemory(rsaBlock))

	// 2. Generate Modern High-Performance Core Key Pair: Ed25519
	_, privKeyEd, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate secure Ed25519 keys: %w", err)
	}
	edBytes, err := x509.MarshalPKCS8PrivateKey(privKeyEd)
	if err != nil {
		return fmt.Errorf("failed to marshal Ed25519 key format: %w", err)
	}
	edBlock := &pem.Block{Type: "PRIVATE KEY", Bytes: edBytes}
	edPEM := string(pem.EncodeToMemory(edBlock))

	actorIRI := fmt.Sprintf("https://%s/actors/%s", domain, actorUUID.String())

	// 3. Commit both verified identities to long-term storage
	err = s.storagePort.CreateActorCredential(ctx, actorIRI, tenantID, "server", rsaPEM, edPEM)
	if err != nil {
		return fmt.Errorf("failed to persist dual-key credentials: %w", err)
	}

	log.Printf("[Bootstrap] Dual-key system actor established successfully: %s", actorIRI)
	return nil
}
