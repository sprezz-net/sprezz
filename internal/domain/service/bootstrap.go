package service

import (
	"context"
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

// provisionServerActor separates identity generation mechanics to reduce cognitive complexity.
func (s *BootstrapService) provisionServerActor(ctx context.Context, domain string, tenantID int32) error {
	log.Printf("[Bootstrap] Generating secure UUIDv4 identity for system actor on domain: %s", domain)

	actorUUID, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("failed to generate random UUIDv4: %w", err)
	}

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate secure RSA keys: %w", err)
	}

	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	}
	privateKeyPEM := string(pem.EncodeToMemory(pemBlock))
	actorIRI := fmt.Sprintf("https://%s/actors/%s", domain, actorUUID.String())

	err = s.storagePort.CreateActorCredential(ctx, actorIRI, tenantID, "server", privateKeyPEM)
	if err != nil {
		return fmt.Errorf("failed to persist system actor credentials for %s: %w", domain, err)
	}

	log.Printf("[Bootstrap] System actor established successfully: %s", actorIRI)
	return nil
}
