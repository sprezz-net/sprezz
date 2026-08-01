package service_test

import (
	"context"
	"testing"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/port/portmock"
	"sprezz/internal/domain/service"

	"github.com/gojuno/minimock/v3"
)

func TestComputeFollowersDigest(t *testing.T) {
	followers := []string{
		"https://remote.com/actor/alice",
		"https://remote.com/actor/bob",
		"https://other.com/actor/charlie",
	}

	// Case 1: Compute for remote.com
	digest1 := service.ComputeFollowersDigest(followers, "remote.com")
	if digest1 == "" {
		t.Error("expected non-empty digest for remote.com")
	}

	// Case 2: Commutative property of XOR (order of followers shouldn't change the digest)
	followersReordered := []string{
		"https://remote.com/actor/bob",
		"https://remote.com/actor/alice",
		"https://other.com/actor/charlie",
	}
	digest2 := service.ComputeFollowersDigest(followersReordered, "remote.com")
	if digest1 != digest2 {
		t.Errorf("XOR digest is not commutative: %s vs %s", digest1, digest2)
	}

	// Case 3: Empty results if no followers on that domain
	digest3 := service.ComputeFollowersDigest(followers, "nonexistent.com")
	if digest3 != "" {
		t.Errorf("expected empty digest for domain with no followers, got %s", digest3)
	}
}

func TestParseCollectionSyncHeader(t *testing.T) {
	headerVal := `collectionId="https://example.org/users/1/followers", url="https://example.org/users/1/followers_synchronization", digest="c33f48cd341ef046"`
	colID, url, digest := service.ParseCollectionSyncHeader(headerVal)

	if colID != "https://example.org/users/1/followers" {
		t.Errorf("expected collectionId to be https://example.org/users/1/followers, got %s", colID)
	}
	if url != "https://example.org/users/1/followers_synchronization" {
		t.Errorf("expected url to be https://example.org/users/1/followers_synchronization, got %s", url)
	}
	if digest != "c33f48cd341ef046" {
		t.Errorf("expected digest to be c33f48cd341ef046, got %s", digest)
	}
}

func TestSyncFollowers_Success(t *testing.T) {
	mc := minimock.NewController(t)

	storage := portmock.NewStorageAndGraphWriterMock(mc)
	fetcher := portmock.NewRemoteFetcherMock(mc)

	actorIRI := "https://local.com/actor/alice"
	remoteCollectionID := "https://remote.com/actor/bob/followers"
	remoteSyncURL := "https://remote.com/actor/bob/followers_synchronization"

	// 1. Mock local followers (alice has charlie, but remote says bob is also following alice, and charlie should be removed)
	storage.StreamQuadsBySubjectMock.Expect(context.Background(), actorIRI).Return([]model.Quad{
		{Subject: actorIRI, Predicate: model.PredicateFollower, Object: "https://remote.com/actor/charlie"},
	}, nil)

	// Mock server credentials resolution
	storage.GetTenantIDByDomainMock.Expect(context.Background(), "local.com").Return(int32(1), nil)
	storage.GetActorCredentialsMock.Expect(context.Background(), int32(1), "server").Return("https://local.com/actor/server", &model.ActorDualKeys{
		PrivateKeyRSAPEM: "rsa-pem",
	}, nil)

	// 2. Mock Remote fetch (remote says bob is following, but charlie is not)
	fetcher.FetchSignedMock.Expect(
		context.Background(),
		remoteSyncURL,
		"https://local.com/actor/server#main-key",
		"rsa-pem",
		"",
	).Return([]byte(`{"type":"OrderedCollection","items":["https://remote.com/actor/bob"]}`), nil)

	// 3. Expect updates in database: add bob, remove charlie
	storage.SaveQuadsMock.Set(func(ctx context.Context, quads []model.Quad) error {
		if len(quads) != 1 {
			t.Errorf("expected 1 quad to save, got %d", len(quads))
		}
		if quads[0].Object != "https://remote.com/actor/bob" {
			t.Errorf("expected follower to add to be bob, got %s", quads[0].Object)
		}
		return nil
	})

	storage.RemoveQuadEdgeMock.Expect(context.Background(), actorIRI, model.PredicateFollower, "https://remote.com/actor/charlie").Return(nil)

	svc := service.NewActivityService(storage, nil, nil, fetcher, service.ActivityServiceConfig{})

	// Calculate a mismatched expected digest so it triggers sync
	expectedDigest := service.ComputeFollowersDigest([]string{"https://remote.com/actor/bob"}, "remote.com")

	err := svc.SyncFollowers(context.Background(), actorIRI, remoteCollectionID, remoteSyncURL, expectedDigest)
	if err != nil {
		t.Fatalf("SyncFollowers failed: %v", err)
	}
}

func TestSyncFollowers_SecurityViolation(t *testing.T) {
	actorIRI := "https://local.com/actor/alice"
	remoteCollectionID := "https://remote.com/actor/bob/followers"
	remoteSyncURL := "https://evil.com/actor/bob/followers_synchronization" // Host mismatch!

	svc := service.NewActivityService(nil, nil, nil, nil, service.ActivityServiceConfig{})

	err := svc.SyncFollowers(context.Background(), actorIRI, remoteCollectionID, remoteSyncURL, "some-digest")
	if err == nil {
		t.Error("expected SSRF host mismatch validation error, got nil")
	}
}

func TestSyncFollowers_CacheUsage(t *testing.T) {
	mc := minimock.NewController(t)

	storage := portmock.NewStorageAndGraphWriterMock(mc)
	cacheMock := portmock.NewFollowersSyncCacheMock(mc)

	actorIRI := "https://local.com/actor/alice"
	remoteCollectionID := "https://remote.com/actor/bob/followers"
	remoteSyncURL := "https://remote.com/actor/bob/followers_synchronization"

	// Mock GetDigest to return a hit matching the expectedDigest
	// Calculate a digest
	expectedDigest := service.ComputeFollowersDigest([]string{"https://remote.com/actor/bob"}, "remote.com")
	cacheMock.GetDigestMock.Expect(context.Background(), actorIRI, "remote.com").Return(expectedDigest, true)

	svc := service.NewActivityService(storage, nil, nil, nil, service.ActivityServiceConfig{
		FollowersSyncCache: cacheMock,
	})

	// Since cache returns expectedDigest, we are instantly in sync! No storage stream or remote fetch is triggered.
	err := svc.SyncFollowers(context.Background(), actorIRI, remoteCollectionID, remoteSyncURL, expectedDigest)
	if err != nil {
		t.Fatalf("SyncFollowers failed: %v", err)
	}
}
