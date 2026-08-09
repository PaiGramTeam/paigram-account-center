package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data/model"
)

func TestArtifactRepoUsesBindingIDForUniqueness(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")
	expiresAt := time.Now().Add(time.Hour)

	require.NoError(t, repo.Put(context.Background(), &biz.Artifact{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		ArtifactType:      "authkey",
		ArtifactValue:     "first-authkey",
		ScopeKey:          "1008611",
		ExpiresAt:         expiresAt,
	}))
	require.NoError(t, repo.Put(context.Background(), &biz.Artifact{
		BindingID:         42,
		PlatformAccountID: "binding_42_20002",
		ArtifactType:      "authkey",
		ArtifactValue:     "second-authkey",
		ScopeKey:          "1008611",
		ExpiresAt:         expiresAt,
	}))

	artifact, err := repo.GetByBindingID(context.Background(), 42, "authkey", "1008611")
	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Equal(t, uint64(42), artifact.BindingID)
	require.Equal(t, "binding_42_20002", artifact.PlatformAccountID)
	require.Equal(t, "second-authkey", artifact.ArtifactValue)

	var count int64
	require.NoError(t, db.Table("runtime_artifacts").Where("binding_id = ? AND artifact_type = ? AND scope_key = ?", 42, "authkey", "1008611").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestArtifactRepoDeleteByBindingIDRemovesOnlyBindingArtifacts(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewArtifactRepo(db, nil, "")
	expiresAt := time.Now().Add(time.Hour)

	require.NoError(t, repo.Put(context.Background(), &biz.Artifact{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		ArtifactType:      "authkey",
		ArtifactValue:     "artifact-42",
		ScopeKey:          "1008611",
		ExpiresAt:         expiresAt,
	}))
	require.NoError(t, repo.Put(context.Background(), &biz.Artifact{
		BindingID:         7,
		PlatformAccountID: "binding_7_20002",
		ArtifactType:      "authkey",
		ArtifactValue:     "artifact-7",
		ScopeKey:          "2008611",
		ExpiresAt:         expiresAt,
	}))

	require.NoError(t, repo.DeleteByBindingID(context.Background(), 42))

	deleted, err := repo.GetByBindingID(context.Background(), 42, "authkey", "1008611")
	require.NoError(t, err)
	require.Nil(t, deleted)

	kept, err := repo.GetByBindingID(context.Background(), 7, "authkey", "2008611")
	require.NoError(t, err)
	require.NotNil(t, kept)
	require.Equal(t, "artifact-7", kept.ArtifactValue)
}

func TestArtifactRepoUsesBindingCacheKeyForBindingOperations(t *testing.T) {
	repo := NewArtifactRepo(nil, nil, "test:")

	require.Equal(t, "test:artifact:binding:42:authkey:1008611", repo.cacheKeyByBinding(42, "authkey", "1008611"))
	require.Equal(t, "test:artifact:binding:42:*", repo.bindingCachePattern(42))
}

func TestArtifactRepoBuildsLegacyAndBindingCacheKeysForDeletedRecords(t *testing.T) {
	repo := NewArtifactRepo(nil, nil, "test:")

	keys := repo.cacheKeysForRecords([]model.RuntimeArtifact{{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		ArtifactType:      "authkey",
		ScopeKey:          "1008611",
	}})

	require.ElementsMatch(t, []string{
		"test:artifact:binding_42_10001:authkey:1008611",
		"test:artifact:binding:42:authkey:1008611",
	}, keys)
}
