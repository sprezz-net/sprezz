package service

import (
	"context"
	"fmt"
	"log"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"

	"github.com/google/uuid"
)

type BootstrapService struct {
	storagePort port.StoragePort
}

func NewBootstrapService(sp port.StoragePort) *BootstrapService {
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
	log.Printf("[Bootstrap] Provisioning dual-key system actor identity for domain: %s", domain)

	actorUUID, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("failed to generate random UUIDv4: %w", err)
	}

	// NEW: Call the centralized key minting function
	newKeys, err := model.MintNewKeyPair()
	if err != nil {
		return fmt.Errorf("failed to mint dual-key pair: %w", err)
	}

	actorIRI := fmt.Sprintf("https://%s/actors/%s", domain, actorUUID.String())

	err = s.storagePort.CreateActorCredential(ctx, actorIRI, tenantID, "server", newKeys.RSAPrivatePEM, newKeys.Ed25519PrivatePEM)
	if err != nil {
		return fmt.Errorf("failed to persist system actor credentials for %s: %w", domain, err)
	}

	log.Printf("[Bootstrap] System actor established successfully: %s", actorIRI)
	return nil
}
