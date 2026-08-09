package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/data/model"
)

func TestGrantInvalidationRepoUpsertStoresMinimumVersion(t *testing.T) {
	db := newRepoTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ConsumerGrantInvalidation{}))
	repo := NewGrantInvalidationRepo(db)

	require.NoError(t, repo.Upsert(context.Background(), 42, "paigram-bot", 3))

	minimum, err := repo.MinimumVersion(context.Background(), 42, "paigram-bot")
	require.NoError(t, err)
	require.Equal(t, uint64(3), minimum)
}

func TestGrantInvalidationRepoUpsertPreservesHighestMinimumVersion(t *testing.T) {
	db := newRepoTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ConsumerGrantInvalidation{}))
	repo := NewGrantInvalidationRepo(db)

	require.NoError(t, repo.Upsert(context.Background(), 42, "paigram-bot", 7))
	require.NoError(t, repo.Upsert(context.Background(), 42, "paigram-bot", 4))

	minimum, err := repo.MinimumVersion(context.Background(), 42, "paigram-bot")
	require.NoError(t, err)
	require.Equal(t, uint64(7), minimum)
}

func TestGrantInvalidationRepoMinimumVersionMissingReturnsZero(t *testing.T) {
	db := newRepoTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ConsumerGrantInvalidation{}))
	repo := NewGrantInvalidationRepo(db)

	minimum, err := repo.MinimumVersion(context.Background(), 42, "paigram-bot")
	require.NoError(t, err)
	require.Zero(t, minimum)
}
