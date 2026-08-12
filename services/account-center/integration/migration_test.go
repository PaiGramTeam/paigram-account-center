//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"paigram/internal/model"
	platformservice "paigram/internal/service/platform"
)

func TestMigrationsApplyToFreshPostgreSQL(t *testing.T) {
	stack := newIntegrationStack(t)

	requireTableExists(t, stack.SQLDB, stack.Schema, "schema_migrations")
	requireTableExists(t, stack.SQLDB, stack.Schema, "users")
	requireTableExists(t, stack.SQLDB, stack.Schema, "user_profiles")
	requireTableExists(t, stack.SQLDB, stack.Schema, "user_emails")
	requireTableExists(t, stack.SQLDB, stack.Schema, "user_sessions")
	requireTableExists(t, stack.SQLDB, stack.Schema, "user_devices")
	requireTableExists(t, stack.SQLDB, stack.Schema, "bot_identities")
	requireTableExists(t, stack.SQLDB, stack.Schema, "bots")
	// Service credentials replace the retired machine identity, signing key,
	// machine token, and opaque bot token stores.
	requireTableExists(t, stack.SQLDB, stack.Schema, "service_credentials")
	requireServiceCredentialColumns(t, stack.SQLDB, stack.Schema)
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "machine_identities")
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "machine_identity_secrets")
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "machine_tokens")
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "signing_keys")
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "bot_tokens")
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "platform_account_refs")
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "bot_account_grants")
	requireTableExists(t, stack.SQLDB, stack.Schema, "platform_services")

	// Bots remain thin identity records without credential or activity state.
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "api_key")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "api_secret")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "scopes")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "metadata")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "last_active_at")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "allow_legacy_binding_write")

	runMigrations(t, stack.SQLDB, stack.DatabaseCfg)
}

func TestUnifiedUserPlatformSchema(t *testing.T) {
	stack := newIntegrationStack(t)

	requireColumnExists(t, stack.SQLDB, stack.Schema, "users", "primary_role_id")
	requireTableExists(t, stack.SQLDB, stack.Schema, "platform_account_bindings")
	requireTableExists(t, stack.SQLDB, stack.Schema, "platform_account_profiles")
	requireTableExists(t, stack.SQLDB, stack.Schema, "consumer_grants")
	requireTableExists(t, stack.SQLDB, stack.Schema, "consumer_grant_actions")
	requireColumnExists(t, stack.SQLDB, stack.Schema, "platform_account_bindings", "active_external_account_marker")
	requireColumnExists(t, stack.SQLDB, stack.Schema, "platform_account_profiles", "primary_profile_marker")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "consumer_grants", "scopes_json")

	requireIndexExists(t, stack.SQLDB, stack.Schema, "users", "idx_users_primary_role_id")
	requireIndexExists(t, stack.SQLDB, stack.Schema, "platform_account_bindings", "uk_platform_account_bindings_active_external_account")
	requireIndexExists(t, stack.SQLDB, stack.Schema, "platform_account_profiles", "uk_platform_account_profiles_primary_per_binding")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "users", "fk_users_primary_role_assignment")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "platform_account_bindings", "fk_platform_account_bindings_owner")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "platform_account_bindings", "fk_platform_account_bindings_primary_profile")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "platform_account_profiles", "fk_platform_account_profiles_binding")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "consumer_grants", "fk_consumer_grants_binding")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "consumer_grants", "fk_consumer_grants_granted_by")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "consumer_grant_actions", "fk_consumer_grant_actions_grant")

	runMigrations(t, stack.SQLDB, stack.DatabaseCfg)
}

func TestUnifiedUserPlatformSchemaConstraints(t *testing.T) {
	stack := newIntegrationStack(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ownerOneID := insertTestUser(t, ctx, stack.SQLDB)
	ownerTwoID := insertTestUser(t, ctx, stack.SQLDB)

	bindingOneID := insertTestBinding(t, ctx, stack.SQLDB, ownerOneID, "mihomo", "cn:1001")
	_, err := stack.SQLDB.ExecContext(ctx, `
		INSERT INTO platform_account_bindings (owner_user_id, platform, external_account_key, platform_service_key, display_name)
		VALUES ($1, $2, $3, 'mihomo', 'Duplicate Active Binding')
	`, ownerTwoID, "mihomo", "cn:1001")
	require.Error(t, err, "expected duplicate active binding to be rejected")

	_, err = stack.SQLDB.ExecContext(ctx, `
		UPDATE platform_account_bindings
		SET deleted_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, bindingOneID)
	require.NoError(t, err)

	recreatedBindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerTwoID, "mihomo", "cn:1001")
	require.NotZero(t, recreatedBindingID, "expected soft-deleted binding to be recreatable")

	primaryProfileID := insertTestProfile(t, ctx, stack.SQLDB, recreatedBindingID, "profile:1", true)
	_, err = stack.SQLDB.ExecContext(ctx, `
		INSERT INTO platform_account_profiles (binding_id, platform_profile_key, game_biz, region, player_uid, nickname, is_primary)
		VALUES ($1, 'profile:2', 'hk4e_cn', 'cn_gf01', '10002', 'Second Primary', TRUE)
	`, recreatedBindingID)
	require.Error(t, err, "expected only one primary profile per binding")

	_, err = stack.SQLDB.ExecContext(ctx, `
		UPDATE platform_account_bindings
		SET primary_profile_id = $1
		WHERE id = $2
	`, primaryProfileID, recreatedBindingID)
	require.NoError(t, err, "expected binding to accept its own profile as primary")

	otherBindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerOneID, "mihomo", "cn:2002")
	_, err = stack.SQLDB.ExecContext(ctx, `
		UPDATE platform_account_bindings
		SET primary_profile_id = $1
		WHERE id = $2
	`, primaryProfileID, otherBindingID)
	require.Error(t, err, "expected binding to reject another binding's profile")
}

func TestIdentityCredentialUniqueIndexesExist(t *testing.T) {
	stack := newIntegrationStack(t)

	requireIndexExists(t, stack.SQLDB, stack.Schema, "user_credentials", "uniq_provider_account")
	requireIndexExists(t, stack.SQLDB, stack.Schema, "user_credentials", "uniq_user_provider")
}

func TestPlatformRegistryDeletionUsesCurrentBindings(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx := context.Background()

	row := model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-service",
		DiscoveryType:        "static",
		Endpoint:             "127.0.0.1:9000",
		Enabled:              true,
		SupportedActionsJSON: `[]`,
		CredentialSchemaJSON: `{}`,
	}
	require.NoError(t, stack.DB.Create(&row).Error)

	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        ownerID,
		Platform:           row.PlatformKey,
		PlatformServiceKey: row.ServiceKey,
		DisplayName:        "Traveler",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, stack.DB.Create(&binding).Error)

	service := platformservice.NewServiceGroup(stack.DB).PlatformService
	require.ErrorIs(t, service.DeletePlatformService(ctx, row.ID), platformservice.ErrPlatformServiceReferenced)

	require.NoError(t, stack.DB.Delete(&binding).Error)
	require.NoError(t, service.DeletePlatformService(ctx, row.ID))
	var deleted model.PlatformService
	require.ErrorIs(t, stack.DB.First(&deleted, row.ID).Error, gorm.ErrRecordNotFound)
}

func insertTestUser(t *testing.T, ctx context.Context, db *sql.DB) uint64 {
	t.Helper()

	var id uint64
	err := db.QueryRowContext(ctx, `INSERT INTO users DEFAULT VALUES RETURNING id`).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertTestBinding(t *testing.T, ctx context.Context, db *sql.DB, ownerUserID uint64, platform, externalAccountKey string) uint64 {
	t.Helper()

	var id uint64
	err := db.QueryRowContext(ctx, `
		INSERT INTO platform_account_bindings (owner_user_id, platform, external_account_key, platform_service_key, display_name)
		VALUES ($1, $2, $3, 'mihomo', 'Binding')
		RETURNING id
	`, ownerUserID, platform, externalAccountKey).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertTestProfile(t *testing.T, ctx context.Context, db *sql.DB, bindingID uint64, profileKey string, isPrimary bool) uint64 {
	t.Helper()

	var id uint64
	err := db.QueryRowContext(ctx, `
		INSERT INTO platform_account_profiles (binding_id, platform_profile_key, game_biz, region, player_uid, nickname, is_primary)
		VALUES ($1, $2, 'hk4e_cn', 'cn_gf01', '10001', 'Primary Profile', $3)
		RETURNING id
	`, bindingID, profileKey, isPrimary).Scan(&id)
	require.NoError(t, err)
	return id
}

func requireColumnExists(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, schema, table, column string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
	`, schema, table, column).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "expected column %s.%s to exist", table, column)
}

func requireIndexExists(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, schema, table, index string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM pg_indexes
		WHERE schemaname = $1 AND tablename = $2 AND indexname = $3
	`, schema, table, index).Scan(&count)
	require.NoError(t, err)
	require.Greater(t, count, 0, "expected index %s on %s to exist", index, table)
}

func requireForeignKeyExists(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, schema, table, constraint string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE table_schema = $1 AND table_name = $2 AND constraint_name = $3 AND constraint_type = 'FOREIGN KEY'
	`, schema, table, constraint).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "expected foreign key %s on %s to exist", constraint, table)
}

// requireTableAbsent confirms a retired table is not present after the
// baseline migration runs.
func requireTableAbsent(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, schema, table string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_name = $2
	`, schema, table).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "expected retired table %s to be absent", table)
}

// requireColumnAbsent verifies that a retired column is gone.
func requireColumnAbsent(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, schema, table, column string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
	`, schema, table, column).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "expected retired column %s.%s to be absent", table, column)
}

// requireServiceCredentialColumns catches stale migration files before the
// gRPC interceptor and token service encounter a column mismatch.
func requireServiceCredentialColumns(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, schema string) {
	t.Helper()
	for _, column := range []string{
		"client_id", "display_name", "secret_hash", "audiences", "scopes",
		"status", "owner_user_id", "description", "last_used_at",
		"created_at", "updated_at", "deleted_at",
	} {
		requireColumnExists(t, db, schema, "service_credentials", column)
	}
}
