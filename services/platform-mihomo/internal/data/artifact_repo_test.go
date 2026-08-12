package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data/model"
)

func TestArtifactRepoUsesBindingRefForUniqueness(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")
	expiresAt := time.Now().Add(time.Hour)

	require.NoError(t, repo.Put(context.Background(), &biz.Artifact{
		BindingRef:    "binding-42",
		AccountKey:    "binding_42_10001",
		ArtifactType:  "authkey",
		ArtifactValue: "first-authkey",
		ScopeKey:      "1008611",
		ExpiresAt:     expiresAt,
	}))
	require.NoError(t, repo.Put(context.Background(), &biz.Artifact{
		BindingRef:    "binding-42",
		AccountKey:    "binding_42_20002",
		ArtifactType:  "authkey",
		ArtifactValue: "second-authkey",
		ScopeKey:      "1008611",
		ExpiresAt:     expiresAt,
	}))

	artifact, err := repo.GetByBindingRef(context.Background(), "binding-42", "authkey", "1008611")
	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Equal(t, "binding-42", artifact.BindingRef)
	require.Equal(t, "binding_42_20002", artifact.AccountKey)
	require.Equal(t, "second-authkey", artifact.ArtifactValue)

	var count int64
	require.NoError(t, db.Table("runtime_artifacts").Where("binding_ref = ? AND artifact_type = ? AND scope_key = ?", "binding-42", "authkey", "1008611").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestArtifactRepoDeleteByBindingRefRemovesOnlyBindingArtifacts(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")
	expiresAt := time.Now().Add(time.Hour)

	require.NoError(t, repo.Put(context.Background(), &biz.Artifact{
		BindingRef:    "binding-42",
		AccountKey:    "binding_42_10001",
		ArtifactType:  "authkey",
		ArtifactValue: "artifact-42",
		ScopeKey:      "1008611",
		ExpiresAt:     expiresAt,
	}))
	require.NoError(t, repo.Put(context.Background(), &biz.Artifact{
		BindingRef:    "binding-7",
		AccountKey:    "binding_7_20002",
		ArtifactType:  "authkey",
		ArtifactValue: "artifact-7",
		ScopeKey:      "2008611",
		ExpiresAt:     expiresAt,
	}))

	require.NoError(t, repo.DeleteByBindingRef(context.Background(), "binding-42"))

	deleted, err := repo.GetByBindingRef(context.Background(), "binding-42", "authkey", "1008611")
	require.NoError(t, err)
	require.Nil(t, deleted)

	kept, err := repo.GetByBindingRef(context.Background(), "binding-7", "authkey", "2008611")
	require.NoError(t, err)
	require.NotNil(t, kept)
	require.Equal(t, "artifact-7", kept.ArtifactValue)
}

func TestArtifactRepoRemovesExpiredArtifactOnRead(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")

	require.NoError(t, repo.Put(context.Background(), &biz.Artifact{
		BindingRef:    "binding-expired",
		AccountKey:    "binding_expired_10001",
		ArtifactType:  "authkey",
		ArtifactValue: "expired-artifact",
		ScopeKey:      "1008611",
		ExpiresAt:     time.Now().Add(-time.Minute),
	}))

	artifact, err := repo.GetByBindingRef(context.Background(), "binding-expired", "authkey", "1008611")
	require.NoError(t, err)
	require.Nil(t, artifact)

	var count int64
	require.NoError(t, db.Table("runtime_artifacts").Where("binding_ref = ?", "binding-expired").Count(&count).Error)
	require.Zero(t, count)
}

func TestArtifactRepoDeleteExpiredKeepsLiveArtifacts(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")
	now := time.Now().UTC()
	for _, artifact := range []*biz.Artifact{
		{BindingRef: "binding-expired", AccountKey: "expired-account", ArtifactType: "authkey", ArtifactValue: "expired", ScopeKey: "1001", ExpiresAt: now.Add(-time.Minute)},
		{BindingRef: "binding-live", AccountKey: "live-account", ArtifactType: "authkey", ArtifactValue: "live", ScopeKey: "2002", ExpiresAt: now.Add(time.Hour)},
	} {
		require.NoError(t, repo.Put(context.Background(), artifact))
	}

	deleted, err := repo.DeleteExpired(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	live, err := repo.GetByBindingRef(context.Background(), "binding-live", "authkey", "2002")
	require.NoError(t, err)
	require.NotNil(t, live)
}

func TestArtifactRepoPutIfCredentialCurrentRejectsStaleGeneration(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")
	credentialRepo := NewCredentialRepo(db)
	require.NoError(t, credentialRepo.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-42", AccountKey: "account-42", Generation: 2, Platform: "mihomo",
		AccountID: "10001", Region: "cn_gf01", CredentialBlob: "ciphertext", CredentialVersion: "v1", Status: "active",
	}))

	err := repo.PutIfCredentialCurrent(context.Background(), &biz.Artifact{
		BindingRef: "binding-42", AccountKey: "account-42", ArtifactType: "authkey",
		ArtifactValue: "artifact", ScopeKey: "1008611", ExpiresAt: time.Now().Add(time.Hour),
	}, 1)
	require.ErrorIs(t, err, biz.ErrArtifactCredentialStale)

	var count int64
	require.NoError(t, db.Table("runtime_artifacts").Count(&count).Error)
	require.Zero(t, count)
}

func TestArtifactRepoPersistsRevocationIntentOutsideTransaction(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")
	intent := &biz.ArtifactRevocationIntent{
		IntentID: "intent-42", BindingRef: "binding-42", AccountKey: "account-42",
		ArtifactType: "authkey", ArtifactValue: "encrypted-authkey", ScopeKey: "1008611",
		ExpiresAt: time.Now().Add(time.Hour), State: "ready", ReadyAfter: time.Now().UTC(),
	}

	persisted, err := repo.PutRevocationIntentImmediately(context.Background(), intent)
	require.NoError(t, err)
	require.Equal(t, intent.IntentID, persisted.IntentID)
	pending, err := repo.HasRevocationPending(context.Background(), "binding-42")
	require.NoError(t, err)
	require.True(t, pending)
	intents, err := repo.ClaimRevocationIntents(context.Background(), time.Now().UTC(), time.Now().UTC().Add(time.Minute), "lease-42")
	require.NoError(t, err)
	require.Len(t, intents, 1)
	require.Equal(t, intent.IntentID, intents[0].IntentID)
	require.Equal(t, intent.BindingRef, intents[0].BindingRef)
	require.Equal(t, intent.ArtifactValue, intents[0].ArtifactValue)
	require.WithinDuration(t, intent.ExpiresAt, intents[0].ExpiresAt, time.Millisecond)

	require.NoError(t, repo.DeleteRevocationIntentImmediately(context.Background(), intent.IntentID))
	pending, err = repo.HasRevocationPending(context.Background(), "binding-42")
	require.NoError(t, err)
	require.False(t, pending)
}

func TestArtifactRepoRevocationDoesNotDeleteSupersedingArtifact(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")
	require.NoError(t, repo.Put(context.Background(), &biz.Artifact{
		BindingRef: "binding-42", AccountKey: "account-42", ArtifactType: "authkey",
		ArtifactValue: "new-ciphertext", ScopeKey: "1008611", ExpiresAt: time.Now().Add(time.Hour),
	}))

	require.NoError(t, repo.DeleteArtifactImmediately(
		context.Background(), "binding-42", "authkey", "1008611", "old-ciphertext",
	))
	artifact, err := repo.GetByBindingRef(context.Background(), "binding-42", "authkey", "1008611")
	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Equal(t, "new-ciphertext", artifact.ArtifactValue)
}

func TestArtifactRepoFinalizeDoesNotDeleteReadyRevocationIntent(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")
	intent := &biz.ArtifactRevocationIntent{
		IntentID: "intent-ready", BindingRef: "binding-42", AccountKey: "account-42",
		ArtifactType: "authkey", ArtifactValue: "encrypted-authkey", ScopeKey: "1008611",
		ExpiresAt: time.Now().Add(time.Hour), State: "ready", ReadyAfter: time.Now().UTC(),
	}
	_, err := repo.PutRevocationIntentImmediately(context.Background(), intent)
	require.NoError(t, err)

	require.ErrorIs(t, repo.FinalizeRevocationIntentImmediately(context.Background(), intent.IntentID), biz.ErrArtifactRevocationPending)
	pending, err := repo.HasRevocationPending(context.Background(), intent.BindingRef)
	require.NoError(t, err)
	require.True(t, pending)
}

func TestArtifactRepoUpgradesDuplicateProvisionalIntentToReady(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")
	provisional := &biz.ArtifactRevocationIntent{
		IntentID: "intent-provisional", BindingRef: "binding-42", AccountKey: "account-42",
		ArtifactType: "authkey", ArtifactValue: "same-ciphertext", ScopeKey: "1008611",
		ExpiresAt: time.Now().Add(time.Hour), State: "provisional", ReadyAfter: time.Now().Add(time.Minute),
	}
	persisted, err := repo.PutRevocationIntentImmediately(context.Background(), provisional)
	require.NoError(t, err)
	require.Equal(t, provisional.IntentID, persisted.IntentID)
	ready := *provisional
	ready.IntentID = "different-random-intent"
	ready.State = "ready"
	ready.ReadyAfter = time.Now().UTC()

	persisted, err = repo.PutRevocationIntentImmediately(context.Background(), &ready)
	require.NoError(t, err)
	require.Equal(t, provisional.IntentID, persisted.IntentID)
	require.Equal(t, "ready", persisted.State)
	require.ErrorIs(t, repo.FinalizeRevocationIntentImmediately(context.Background(), provisional.IntentID), biz.ErrArtifactRevocationPending)
}

func TestArtifactRepoUsesBindingCacheKeyForBindingOperations(t *testing.T) {
	repo := NewArtifactRepo(nil, nil, "test:")

	require.Equal(t, "test:artifact:binding:binding-42:authkey:1008611", repo.cacheKeyByBinding("binding-42", "authkey", "1008611"))
	require.Equal(t, "test:artifact:binding:binding-42:*", repo.bindingCachePattern("binding-42"))
}

func TestArtifactRepoBuildsLegacyAndBindingCacheKeysForDeletedRecords(t *testing.T) {
	repo := NewArtifactRepo(nil, nil, "test:")

	keys := repo.cacheKeysForRecords([]model.RuntimeArtifact{{
		BindingRef:   "binding-42",
		AccountKey:   "binding_42_10001",
		ArtifactType: "authkey",
		ScopeKey:     "1008611",
	}})

	require.ElementsMatch(t, []string{
		"test:artifact:binding_42_10001:authkey:1008611",
		"test:artifact:binding:binding-42:authkey:1008611",
	}, keys)
}
