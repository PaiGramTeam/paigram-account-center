package data

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"platform-mihomo-service/internal/biz"
)

func TestCredentialRepoGetByBindingIDUsesUniqueBindingRecord(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewCredentialRepo(db)
	now := time.Now().UTC()

	require.NoError(t, repo.Save(context.Background(), &biz.Credential{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		Platform:          "mihomo",
		AccountID:         "10001",
		Region:            "cn_gf01",
		CredentialBlob:    "blob-1",
		CredentialVersion: "v1",
		Status:            "active",
		LastValidatedAt:   &now,
	}))
	require.NoError(t, repo.Save(context.Background(), &biz.Credential{
		BindingID:         42,
		PlatformAccountID: "binding_42_20002",
		Platform:          "mihomo",
		AccountID:         "20002",
		Region:            "cn_gf01",
		CredentialBlob:    "blob-2",
		CredentialVersion: "v1",
		Status:            "active",
		LastValidatedAt:   &now,
	}))

	credential, err := repo.GetByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, "binding_42_20002", credential.PlatformAccountID)
	require.Equal(t, "20002", credential.AccountID)

	var count int64
	require.NoError(t, db.Table("credential_records").Where("binding_id = ?", 42).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func newRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", dbName, time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE credential_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		binding_id INTEGER NOT NULL,
		platform_account_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		account_id TEXT NOT NULL,
		region TEXT NOT NULL,
		credential_blob TEXT NOT NULL,
		credential_version TEXT NOT NULL,
		status TEXT NOT NULL,
		last_validated_at DATETIME NULL,
		last_refreshed_at DATETIME NULL,
		expires_at DATETIME NULL,
		created_at DATETIME NULL,
		updated_at DATETIME NULL,
		UNIQUE(binding_id),
		UNIQUE(platform_account_id),
		UNIQUE(platform, account_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE account_profiles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		binding_id INTEGER NOT NULL,
		platform_account_id TEXT NOT NULL,
		game_biz TEXT NOT NULL,
		region TEXT NOT NULL,
		player_id TEXT NOT NULL,
		nickname TEXT NOT NULL,
		level INTEGER NOT NULL DEFAULT 0,
		is_default NUMERIC NOT NULL DEFAULT 0,
		default_profile_marker INTEGER GENERATED ALWAYS AS (CASE WHEN is_default = 1 THEN 1 ELSE NULL END) STORED,
		discovered_at DATETIME NOT NULL,
		updated_at DATETIME NULL,
		UNIQUE(binding_id, player_id, region),
		UNIQUE(binding_id, default_profile_marker)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE device_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		binding_id INTEGER NOT NULL,
		platform_account_id TEXT NOT NULL,
		device_id TEXT NOT NULL,
		device_fp TEXT NOT NULL,
		device_name TEXT NULL,
		is_valid NUMERIC NOT NULL DEFAULT 1,
		last_seen_at DATETIME NULL,
		created_at DATETIME NULL,
		updated_at DATETIME NULL,
		UNIQUE(binding_id, device_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE runtime_artifacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		binding_id INTEGER NOT NULL,
		platform_account_id TEXT NOT NULL,
		artifact_type TEXT NOT NULL,
		artifact_value TEXT NOT NULL,
		scope_key TEXT NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NULL,
		updated_at DATETIME NULL,
		UNIQUE(binding_id, artifact_type, scope_key)
	)`).Error)
	return db
}
