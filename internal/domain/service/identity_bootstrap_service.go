package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port"
	"sprezz/internal/pkg/cryptoutil"

	"github.com/google/uuid"
)

type BootstrapService struct {
	storagePort port.StoragePort
}

func NewBootstrapService(sp port.StoragePort) *BootstrapService {
	return &BootstrapService{storagePort: sp}
}

// BootstrapTenantsAndServerActors checks and maps configured domains cleanly.
func (s *BootstrapService) BootstrapTenantsAndServerActors(ctx context.Context, configuredTenants map[string]string) (map[string]int32, error) {
	tenantMap := make(map[string]int32)
	for tUUID, domain := range configuredTenants {
		tenantID, err := s.storagePort.UpsertConfiguredTenant(ctx, tUUID, domain)
		if err != nil {
			return nil, fmt.Errorf("failed to reconcile tenant for domain %s: %w", domain, err)
		}

		tenantMap[domain] = tenantID

		exists, err := s.storagePort.HasActorCredential(ctx, tenantID, "server")
		if err != nil {
			return nil, fmt.Errorf("failed to verify server actor status for %s: %w", domain, err)
		}

		if !exists {
			if err := s.provisionServerActor(ctx, domain, tenantID); err != nil {
				return nil, err
			}
		}
	}
	return tenantMap, nil
}

func (s *BootstrapService) provisionServerActor(ctx context.Context, domain string, tenantID int32) error {
	log.Printf("[Bootstrap] Provisioning dual-key system actor identity for domain: %s", domain)

	actorUUID, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("failed to generate random UUIDv4: %w", err)
	}

	// NEW: Call the centralized key minting function
	newKeys, err := cryptoutil.MintNewKeyPair()
	if err != nil {
		return fmt.Errorf("failed to mint dual-key pair: %w", err)
	}

	actorIRI := model.ActorIRI(domain, actorUUID.String())

	err = s.storagePort.CreateActorCredential(ctx, actorIRI, tenantID, "server", newKeys.RSAPrivatePEM, newKeys.Ed25519PrivatePEM)
	if err != nil {
		return fmt.Errorf("failed to persist system actor credentials for %s: %w", domain, err)
	}

	// Derive public key block for the RDF record
	pubKey, err := cryptoutil.ExtractRSAPublicKey(newKeys.RSAPrivatePEM)
	if err != nil {
		return fmt.Errorf("failed to extract public key: %w", err)
	}

	// Create a graph version payload block for the actor profile
	const inboxSuffix = "/inbox"
	payloadMap := map[string]interface{}{
		"@context": []interface{}{
			"https://www.w3.org/ns/activitystreams",
			"https://purl.archive.org/socialweb/pending/1",
			"https://purl.archive.org/socialweb/blocked",
		},
		"id":                        actorIRI,
		"type":                      "Application",
		"preferredUsername":         "server",
		"inbox":                     actorIRI + inboxSuffix,
		model.ShortFollowers:        actorIRI + "/" + model.ShortFollowers,
		model.ShortFollowing:        actorIRI + "/" + model.ShortFollowing,
		model.ShortPendingFollowers: actorIRI + "/" + model.ShortPendingFollowers,
		model.ShortPendingFollowing: actorIRI + "/" + model.ShortPendingFollowing,
		model.ShortBlocked:          actorIRI + "/" + model.ShortBlocked,
		model.ShortBlocks:           actorIRI + "/" + model.ShortBlocks,
		"endpoints": map[string]interface{}{
			"sharedInbox": "https://" + domain + inboxSuffix,
		},
		"publicKey": map[string]interface{}{
			"id":           actorIRI + model.SuffixMainKey,
			"owner":        actorIRI,
			"publicKeyPem": pubKey,
		},
	}
	payloadBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return fmt.Errorf("failed to marshal server actor profile: %w", err)
	}

	graphID, err := s.storagePort.CreateGraphVersion(ctx, actorIRI, actorIRI, payloadBytes)
	if err != nil {
		return fmt.Errorf("failed to register server actor graph version: %w", err)
	}

	// Write semantic RDF triples directly into the quad store
	quads := []model.Quad{
		{
			GraphID:   graphID,
			Subject:   actorIRI,
			Predicate: model.RDFType,
			Object:    model.ActorApplication,
			ObjType:   model.NamedNode,
		},
		{
			GraphID:   graphID,
			Subject:   actorIRI,
			Predicate: model.PredicatePreferredUsername,
			Object:    "server",
			ObjType:   model.Literal,
		},
		{
			GraphID:   graphID,
			Subject:   actorIRI,
			Predicate: model.PredicatePublicKeyPem,
			Object:    pubKey,
			ObjType:   model.Literal,
		},
		{
			GraphID:   graphID,
			Subject:   actorIRI,
			Predicate: "https://www.w3.org/ns/activitystreams#inbox",
			Object:    actorIRI + inboxSuffix,
			ObjType:   model.NamedNode,
		},
		{
			GraphID:   graphID,
			Subject:   actorIRI,
			Predicate: "https://www.w3.org/ns/activitystreams#sharedInbox",
			Object:    "https://" + domain + inboxSuffix,
			ObjType:   model.NamedNode,
		},
	}
	if err := s.storagePort.SaveQuads(ctx, quads); err != nil {
		return fmt.Errorf("failed to save server actor semantic triples: %w", err)
	}

	log.Printf("[Bootstrap] System actor established and semantic triples persisted successfully: %s", actorIRI)
	return nil
}
