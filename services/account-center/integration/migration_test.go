//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	// Path D §2.1 + §2.2: service_credentials replaces machine_identities,
	// machine_identity_secrets, machine_tokens, signing_keys, and the
	// legacy bot_tokens opaque store. Confirm the new table exists and
	// the five old tables do not.
	requireTableExists(t, stack.SQLDB, stack.Schema, "service_credentials")
	requireServiceCredentialColumns(t, stack.SQLDB, stack.Schema)
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "machine_identities")
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "machine_identity_secrets")
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "machine_tokens")
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "signing_keys")
	requireTableAbsent(t, stack.SQLDB, stack.Schema, "bot_tokens")
	requireTableExists(t, stack.SQLDB, stack.Schema, "platform_account_refs")
	requireTableExists(t, stack.SQLDB, stack.Schema, "bot_account_grants")
	requireTableExists(t, stack.SQLDB, stack.Schema, "platform_services")

	// Path D §10 Q5: bots stays as a thin identity table, without the
	// legacy api_key/api_secret/scopes/metadata/last_active_at columns.
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "api_key")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "api_secret")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "scopes")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "metadata")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "bots", "last_active_at")

	runMigrations(t, stack.SQLDB, stack.DatabaseCfg)
}

func TestUnifiedUserPlatformSchema(t *testing.T) {
	stack := newIntegrationStack(t)

	requireColumnExists(t, stack.SQLDB, stack.Schema, "users", "primary_role_id")
	requireTableExists(t, stack.SQLDB, stack.Schema, "platform_account_bindings")
	requireTableExists(t, stack.SQLDB, stack.Schema, "platform_account_profiles")
	requireTableExists(t, stack.SQLDB, stack.Schema, "consumer_grants")
	requireColumnExists(t, stack.SQLDB, stack.Schema, "platform_account_bindings", "active_external_account_marker")
	requireColumnExists(t, stack.SQLDB, stack.Schema, "platform_account_profiles", "primary_profile_marker")

	requireIndexExists(t, stack.SQLDB, stack.Schema, "users", "idx_users_primary_role_id")
	requireIndexExists(t, stack.SQLDB, stack.Schema, "platform_account_bindings", "uk_platform_account_bindings_active_external_account")
	requireIndexExists(t, stack.SQLDB, stack.Schema, "platform_account_profiles", "uk_platform_account_profiles_primary_per_binding")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "users", "fk_users_primary_role_assignment")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "platform_account_bindings", "fk_platform_account_bindings_owner")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "platform_account_bindings", "fk_platform_account_bindings_primary_profile")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "platform_account_profiles", "fk_platform_account_profiles_binding")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "consumer_grants", "fk_consumer_grants_binding")
	requireForeignKeyExists(t, stack.SQLDB, stack.Schema, "consumer_grants", "fk_consumer_grants_granted_by")

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

// requireTableAbsent confirms a Path D-deprecated table is not present
// after the rewritten 000001 migration runs (Path D §2.2). The exact
// counterpart to requireTableExists.
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
	require.Equal(t, 0, count, "expected Path-D-dropped table %s to be absent", table)
}

// requireColumnAbsent verifies a Path D-dropped column is gone. Used to
// confirm the thin bots schema (Path D §10 Q5).
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
	require.Equal(t, 0, count, "expected Path-D-dropped column %s.%s to be absent", table, column)
}

// requireServiceCredentialColumns verifies the column set added by the
// Path D §2.1 service_credentials DDL. Catches stale migration files
// before the gRPC interceptor + token service hit a column-mismatch
// error at request time.
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
