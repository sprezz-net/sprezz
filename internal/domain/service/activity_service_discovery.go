package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sprezz/internal/domain/model"
	"sprezz/internal/pkg/httputil"
)

func (s *ActivityService) resolveServerActorInbox(ctx context.Context, targetDomain string, senderActorIRI string) (string, error) {
	// 1. Try local cache first
	if cachedInbox := s.resolveCachedServerInbox(ctx, targetDomain); cachedInbox != "" {
		return cachedInbox, nil
	}

	// 2. Discover via signed FEP-d556 process (Signed with dual-keys)
	serverActorIRI, serverKeys, err := s.getLocalServerCredentials(ctx, senderActorIRI)
	if err != nil {
		return "", err
	}

	targetKeyID := serverActorIRI + model.SuffixMainKey

	// Step A: WebFinger query for resource=https://<domain>
	actorProfileURL, err := s.discoverRemoteActorIRI(ctx, targetDomain, targetKeyID, serverKeys.PrivateKeyRSAPEM, serverKeys.PrivateKeyEd25519PEM)
	if err != nil {
		return "", err
	}

	// Step B: Fetch the actor profile
	resolvedSharedInbox, profileBody, err := s.discoverRemoteSharedInbox(ctx, actorProfileURL, targetKeyID, serverKeys.PrivateKeyRSAPEM, serverKeys.PrivateKeyEd25519PEM)
	if err != nil {
		return "", err
	}

	// Cache the resolved inbox
	s.cacheServerInbox(ctx, targetDomain, resolvedSharedInbox, profileBody)

	return resolvedSharedInbox, nil
}

// resolveCachedServerInbox queries our database for a cached server actor's shared inbox URL
func (s *ActivityService) resolveCachedServerInbox(ctx context.Context, targetDomain string) string {
	inboxURL, err := s.resolveActorInbox(ctx, httputil.HTTPSPrefix+targetDomain)
	if err == nil && inboxURL != "" {
		return inboxURL
	}
	inboxURL, err = s.resolveActorInbox(ctx, httputil.HTTPPrefix+targetDomain)
	if err == nil && inboxURL != "" {
		return inboxURL
	}
	return ""
}

// getLocalServerCredentials resolves the local tenant and system server credentials (RSA + Ed25519)
func (s *ActivityService) getLocalServerCredentials(ctx context.Context, senderActorIRI string) (string, *model.ActorDualKeys, error) {
	senderHost := extractDomain(senderActorIRI)
	if senderHost == "" {
		return "", nil, fmt.Errorf("invalid sender actor IRI format")
	}

	tenantID, err := s.storage.GetTenantIDByDomain(ctx, senderHost)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get tenant ID: %w", err)
	}

	serverActorIRI, serverKeys, err := s.storage.GetActorCredentials(ctx, tenantID, "server")
	if err != nil {
		return "", nil, fmt.Errorf("failed to load local server actor credentials: %w", err)
	}

	return serverActorIRI, serverKeys, nil
}

func (s *ActivityService) parseWebFingerSelfLink(wfRespBody []byte) (string, error) {
	var wfResp struct {
		Links []struct {
			Rel  string `json:"rel"`
			Type string `json:"type"`
			Href string `json:"href"`
		} `json:"links"`
	}
	if err := json.Unmarshal(wfRespBody, &wfResp); err != nil {
		return "", fmt.Errorf("failed to parse webfinger response: %w", err)
	}

	for _, link := range wfResp.Links {
		if link.Rel == "self" && (strings.Contains(link.Type, "activity") || strings.Contains(link.Type, "json")) {
			return link.Href, nil
		}
	}
	return "", fmt.Errorf("no self link found in webfinger response")
}

func (s *ActivityService) fetchWebFingerJRD(ctx context.Context, targetDomain, keyID, privateKeyRSAPEM, privateKeyEd25519PEM string) ([]byte, error) {
	webfingerURL := fmt.Sprintf("https://%s/.well-known/webfinger?resource=https://%s", targetDomain, targetDomain)
	wfBody, err := s.fetcher.FetchSigned(ctx, webfingerURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM)
	if err != nil {
		webfingerURL = fmt.Sprintf("http://%s/.well-known/webfinger?resource=http://%s", targetDomain, targetDomain)
		wfBody, err = s.fetcher.FetchSigned(ctx, webfingerURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM)
		if err != nil {
			return nil, fmt.Errorf("webfinger discovery failed: %w", err)
		}
	}
	return wfBody, nil
}

// discoverRemoteActorIRI queries the WebFinger JRD of a remote domain to resolve its server-controlled actor IRI
func (s *ActivityService) discoverRemoteActorIRI(ctx context.Context, targetDomain, keyID, privateKeyRSAPEM, privateKeyEd25519PEM string) (string, error) {
	wfBody, err := s.fetchWebFingerJRD(ctx, targetDomain, keyID, privateKeyRSAPEM, privateKeyEd25519PEM)
	if err != nil {
		return "", err
	}
	return s.parseWebFingerSelfLink(wfBody)
}

// discoverRemoteSharedInbox fetches and parses a remote actor's JSON-LD profile to extract its shared inbox URL
func (s *ActivityService) discoverRemoteSharedInbox(ctx context.Context, actorProfileURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM string) (string, []byte, error) {
	profileBody, err := s.fetcher.FetchSigned(ctx, actorProfileURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM)
	if err != nil {
		return "", nil, fmt.Errorf("failed to fetch server actor profile: %w", err)
	}

	var profile struct {
		Inbox       string `json:"inbox"`
		SharedInbox string `json:"sharedInbox"`
		Endpoints   struct {
			SharedInbox string `json:"sharedInbox"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(profileBody, &profile); err != nil {
		return "", nil, fmt.Errorf("failed to parse actor profile: %w", err)
	}

	resolvedSharedInbox := profile.Endpoints.SharedInbox
	if resolvedSharedInbox == "" {
		resolvedSharedInbox = profile.SharedInbox
	}
	if resolvedSharedInbox == "" {
		resolvedSharedInbox = profile.Inbox
	}

	if resolvedSharedInbox == "" {
		return "", nil, fmt.Errorf("no shared inbox or inbox found in server actor profile")
	}

	return resolvedSharedInbox, profileBody, nil
}

// cacheServerInbox persists the discovered shared inbox as quads so future queries are instant
func (s *ActivityService) cacheServerInbox(ctx context.Context, targetDomain, sharedInbox string, profileBody []byte) {
	iri := httputil.HTTPSPrefix + targetDomain
	graphID, _ := s.storage.CreateGraphVersion(ctx, iri, iri, profileBody)
	if graphID > 0 {
		quads := []model.Quad{
			{
				GraphID:   graphID,
				Subject:   iri,
				Predicate: "https://www.w3.org/ns/activitystreams#sharedInbox",
				Object:    sharedInbox,
				ObjType:   model.NamedNode,
			},
		}
		_ = s.storage.SaveQuads(ctx, quads)
	}
}
