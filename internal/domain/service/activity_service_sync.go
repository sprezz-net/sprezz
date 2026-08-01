package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"sprezz/internal/domain/model"
)

// ComputeFollowersDigest calculates the FEP-8fcf partial followers collection digest.
// It computes the XOR sum of SHA256 hashes of all follower IDs that belong to targetDomain.
func ComputeFollowersDigest(followers []string, targetDomain string) string {
	var digest [32]byte
	hasFollowers := false
	for _, follower := range followers {
		if extractDomain(follower) == targetDomain {
			h := sha256.Sum256([]byte(follower))
			if !hasFollowers {
				digest = h
				hasFollowers = true
			} else {
				for i := 0; i < 32; i++ {
					digest[i] ^= h[i]
				}
			}
		}
	}
	if !hasFollowers {
		return ""
	}
	return hex.EncodeToString(digest[:])
}

// GetPartialFollowers returns the subset of followers whose IRI belongs to targetDomain.
func GetPartialFollowers(followers []string, targetDomain string) []string {
	var partial []string
	for _, f := range followers {
		if extractDomain(f) == targetDomain {
			partial = append(partial, f)
		}
	}
	return partial
}

// ParseCollectionSyncHeader parses standard HTTP-Signatures parameter format in FEP-8fcf header.
func ParseCollectionSyncHeader(headerVal string) (collectionID, syncURL, digest string) {
	params := make(map[string]string)
	parts := strings.Split(headerVal, ",")
	for _, part := range parts {
		subParts := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(subParts) == 2 {
			k := strings.TrimSpace(subParts[0])
			v := strings.Trim(strings.TrimSpace(subParts[1]), "\"`")
			params[k] = v
		}
	}
	return params["collectionId"], params["url"], params["digest"]
}

// getOrComputeFollowersDigest fetches digest from cache or computes and caches it.
func (s *ActivityService) getOrComputeFollowersDigest(ctx context.Context, actorIRI, targetDomain string) string {
	if s.syncCache != nil {
		if cached, found := s.syncCache.GetDigest(ctx, actorIRI, targetDomain); found {
			return cached
		}
	}

	followers, err := s.GetFollowersTimeline(ctx, actorIRI, 10000, 0)
	if err != nil {
		return ""
	}

	digest := ComputeFollowersDigest(followers, targetDomain)
	if s.syncCache != nil && digest != "" {
		s.syncCache.SetDigest(ctx, actorIRI, targetDomain, digest)
	}
	return digest
}

// evictFollowersDigest invalidates cached digest entry for actor & remote domain.
func (s *ActivityService) evictFollowersDigest(ctx context.Context, actorIRI, targetDomain string) {
	if s.syncCache != nil {
		s.syncCache.EvictDigest(ctx, actorIRI, targetDomain)
	}
}

// fetchRemoteFollowers retrieves and parses remote synchronization collection.
func (s *ActivityService) fetchRemoteFollowers(ctx context.Context, actorIRI, remoteSyncURL string) ([]string, error) {
	serverActorIRI, keys, err := s.getLocalServerCredentials(ctx, actorIRI)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local server credentials for sync: %w", err)
	}

	body, err := s.fetcher.FetchSigned(ctx, remoteSyncURL, serverActorIRI+model.SuffixMainKey, keys.PrivateKeyRSAPEM, keys.PrivateKeyEd25519PEM)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote synchronization collection from %s: %w", remoteSyncURL, err)
	}

	var collection struct {
		Type         string   `json:"type"`
		Items        []string `json:"items"`
		OrderedItems []string `json:"orderedItems"`
	}
	if err := json.Unmarshal(body, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse remote synchronization collection: %w", err)
	}

	remoteFollowers := collection.Items
	if len(remoteFollowers) == 0 {
		remoteFollowers = collection.OrderedItems
	}
	return remoteFollowers, nil
}

// reconcileFollowers aligns our local follower quads with the remote authoritative list.
func (s *ActivityService) reconcileFollowers(ctx context.Context, actorIRI string, localPartial, remoteFollowers []string) error {
	localMap := make(map[string]struct{})
	for _, f := range localPartial {
		localMap[f] = struct{}{}
	}

	remoteMap := make(map[string]struct{})
	for _, f := range remoteFollowers {
		remoteMap[f] = struct{}{}
	}

	// ADD missing followers (remote says yes, local says no)
	for _, f := range remoteFollowers {
		if _, exists := localMap[f]; !exists {
			quads := []model.Quad{
				{
					Subject:   actorIRI,
					Predicate: model.PredicateFollower,
					Object:    f,
					ObjType:   model.NamedNode,
				},
			}
			if err := s.storage.SaveQuads(ctx, quads); err != nil {
				return fmt.Errorf("failed to add follower %s: %w", f, err)
			}
		}
	}

	// REMOVE stale followers (local says yes, remote says no)
	for _, f := range localPartial {
		if _, exists := remoteMap[f]; !exists {
			if err := s.storage.RemoveQuadEdge(ctx, actorIRI, model.PredicateFollower, f); err != nil {
				return fmt.Errorf("failed to remove follower %s: %w", f, err)
			}
		}
	}

	return nil
}

// SyncFollowers performs FEP-8fcf followers collection synchronization.
// It compares local followers for targetDomain against remote state, and updates them if needed.
func (s *ActivityService) SyncFollowers(ctx context.Context, actorIRI, remoteCollectionID, remoteSyncURL, expectedDigest string) error {
	syncHost := extractDomain(remoteSyncURL)
	collectionHost := extractDomain(remoteCollectionID)
	if syncHost == "" || collectionHost == "" || syncHost != collectionHost {
		return fmt.Errorf("FEP-8fcf security violation: sync URL host %q does not match collection host %q", syncHost, collectionHost)
	}

	// Use our high-performance caching wrapper instead of direct DB queries
	localDigest := s.getOrComputeFollowersDigest(ctx, actorIRI, syncHost)
	if localDigest == expectedDigest {
		return nil
	}

	remoteFollowers, err := s.fetchRemoteFollowers(ctx, actorIRI, remoteSyncURL)
	if err != nil {
		return err
	}

	followers, err := s.GetFollowersTimeline(ctx, actorIRI, 10000, 0)
	if err != nil {
		return fmt.Errorf("failed to get local followers for sync: %w", err)
	}

	localPartial := GetPartialFollowers(followers, syncHost)

	if err := s.reconcileFollowers(ctx, actorIRI, localPartial, remoteFollowers); err != nil {
		return err
	}

	// Evict cached entry on successful sync to force recalculation on next read
	s.evictFollowersDigest(ctx, actorIRI, syncHost)

	return nil
}
