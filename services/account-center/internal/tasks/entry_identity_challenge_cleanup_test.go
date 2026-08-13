package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCleanExpiredEntryIdentityChallengesDeletesOnlyExpiredRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:entry-identity-cleanup?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE entry_identity_link_challenges (
		challenge_hash TEXT PRIMARY KEY,
		expires_at DATETIME NOT NULL
	)`).Error)
	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO entry_identity_link_challenges (challenge_hash, expires_at) VALUES (?, ?), (?, ?)`,
		"expired", now.Add(-time.Minute), "pending", now.Add(time.Minute),
	).Error)

	err = NewCleanExpiredEntryIdentityChallengesHandler(db).ProcessTask(context.Background(), nil)
	require.NoError(t, err)

	var hashes []string
	require.NoError(t, db.Table("entry_identity_link_challenges").Order("challenge_hash").Pluck("challenge_hash", &hashes).Error)
	require.Equal(t, []string{"pending"}, hashes)
}
