package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestManagementRepoDeleteCredentialGraphByBindingIDRemovesBindingRows(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewManagementRepo(db, nil, "")
	now := time.Now().UTC()

	require.NoError(t, db.Exec(`INSERT INTO credential_records (binding_id, platform_account_id, platform, account_id, region, credential_blob, credential_version, status) VALUES
		(42, 'binding_42_10001', 'mihomo', '10001', 'cn_gf01', 'blob-42', 'v1', 'active'),
		(7, 'binding_7_20002', 'mihomo', '20002', 'cn_gf01', 'blob-7', 'v1', 'active')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO device_records (binding_id, platform_account_id, device_id, device_fp, is_valid) VALUES
		(42, 'binding_42_10001', 'device-42', 'fp-42', 1),
		(7, 'binding_7_20002', 'device-7', 'fp-7', 1)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO account_profiles (binding_id, platform_account_id, game_biz, region, player_id, nickname, level, is_default, discovered_at) VALUES
		(42, 'binding_42_10001', 'hk4e_cn', 'cn_gf01', '1008611', 'Traveler', 60, 1, ?),
		(7, 'binding_7_20002', 'hk4e_cn', 'cn_gf01', '2008611', 'Other', 50, 1, ?)`, now, now).Error)
	require.NoError(t, db.Exec(`INSERT INTO runtime_artifacts (binding_id, platform_account_id, artifact_type, artifact_value, scope_key, expires_at) VALUES
		(42, 'binding_42_10001', 'authkey', 'artifact-42', '1008611', ?),
		(7, 'binding_7_20002', 'authkey', 'artifact-7', '2008611', ?)`, now.Add(time.Hour), now.Add(time.Hour)).Error)

	require.NoError(t, repo.DeleteCredentialGraphByBindingID(context.Background(), 42))

	requireTableCount(t, db, "credential_records", "binding_id = ?", 42, 0)
	requireTableCount(t, db, "device_records", "binding_id = ?", 42, 0)
	requireTableCount(t, db, "account_profiles", "binding_id = ?", 42, 0)
	requireTableCount(t, db, "runtime_artifacts", "platform_account_id = ?", "binding_42_10001", 0)
	requireTableCount(t, db, "credential_records", "binding_id = ?", 7, 1)
	requireTableCount(t, db, "device_records", "binding_id = ?", 7, 1)
	requireTableCount(t, db, "account_profiles", "binding_id = ?", 7, 1)
	requireTableCount(t, db, "runtime_artifacts", "platform_account_id = ?", "binding_7_20002", 1)
}

func TestManagementRepoDeleteCredentialGraphByPlatformAccountIDRemovesResolvedBindingArtifacts(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewManagementRepo(db, nil, "")
	now := time.Now().UTC()

	require.NoError(t, db.Exec(`INSERT INTO credential_records (binding_id, platform_account_id, platform, account_id, region, credential_blob, credential_version, status) VALUES
		(42, 'binding_42_20002', 'mihomo', '20002', 'cn_gf01', 'blob-42', 'v1', 'active'),
		(7, 'binding_7_30003', 'mihomo', '30003', 'cn_gf01', 'blob-7', 'v1', 'active')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO account_profiles (binding_id, platform_account_id, game_biz, region, player_id, nickname, level, is_default, discovered_at) VALUES
		(42, 'binding_42_20002', 'hk4e_cn', 'cn_gf01', '1008611', 'Traveler', 60, 1, ?),
		(7, 'binding_7_30003', 'hk4e_cn', 'cn_gf01', '3008611', 'Other', 50, 1, ?)`, now, now).Error)
	require.NoError(t, db.Exec(`INSERT INTO runtime_artifacts (binding_id, platform_account_id, artifact_type, artifact_value, scope_key, expires_at) VALUES
		(42, 'binding_42_10001', 'authkey', 'stale-platform-artifact', '1008611', ?),
		(7, 'binding_7_30003', 'authkey', 'artifact-7', '3008611', ?)`, now.Add(time.Hour), now.Add(time.Hour)).Error)

	require.NoError(t, repo.DeleteCredentialGraph(context.Background(), "binding_42_20002"))

	requireTableCount(t, db, "credential_records", "binding_id = ?", 42, 0)
	requireTableCount(t, db, "account_profiles", "binding_id = ?", 42, 0)
	requireTableCount(t, db, "runtime_artifacts", "binding_id = ?", 42, 0)
	requireTableCount(t, db, "credential_records", "binding_id = ?", 7, 1)
	requireTableCount(t, db, "account_profiles", "binding_id = ?", 7, 1)
	requireTableCount(t, db, "runtime_artifacts", "binding_id = ?", 7, 1)
}

func requireTableCount(t *testing.T, db *gorm.DB, table string, query string, arg any, want int64) {
	t.Helper()
	var count int64
	require.NoError(t, db.Table(table).Where(query, arg).Count(&count).Error)
	require.Equal(t, want, count)
}
