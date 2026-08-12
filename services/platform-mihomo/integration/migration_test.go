//go:build integration

package integration

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestMigrationsCreateV2Baseline(t *testing.T) {
	stack := newEmptyIntegrationStack(t)
	requireMigrationsApplied(t, stack.SQLDB)

	for _, table := range []string{
		"credential_records",
		"device_records",
		"account_profiles",
		"runtime_artifacts",
		"consumer_grant_invalidations",
		"authorization_fences",
		"platform_operations",
	} {
		requireTableExists(t, stack.SQLDB, "public", table)
	}
}

func execMigrationFile(db *sql.DB, path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = db.Exec(string(contents))
	return err
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration test path")
	}
	return filepath.Dir(filepath.Dir(currentFile))
}

func requireTableExists(t *testing.T, db *sql.DB, schema string, table string) {
	t.Helper()
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = $2
		)
	`, schema, table).Scan(&exists)
	if err != nil {
		t.Fatalf("query table %s.%s: %v", schema, table, err)
	}
	if !exists {
		t.Fatalf("expected table %s.%s to exist", schema, table)
	}
}

func TestV2BaselineRejectsMismatchedBindingAccountPair(t *testing.T) {
	stack := newEmptyIntegrationStack(t)
	requireMigrationsApplied(t, stack.SQLDB)
	insertCredential(t, stack.SQLDB, "binding-a", "account-a", "10001")
	insertCredential(t, stack.SQLDB, "binding-b", "account-b", "20002")

	_, err := stack.SQLDB.Exec(`
		INSERT INTO account_profiles (
			binding_ref, account_key, profile_ref, game_biz, region, player_id, nickname
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "binding-a", "account-b", "profile-a", "hk4e_cn", "cn_gf01", "1008611", "Traveler")
	if err == nil {
		t.Fatal("expected mismatched binding/account pair to violate the foreign key")
	}
}

func TestV2BaselineRejectsInvalidOperationGeneration(t *testing.T) {
	stack := newEmptyIntegrationStack(t)
	requireMigrationsApplied(t, stack.SQLDB)

	_, err := stack.SQLDB.Exec(`
		INSERT INTO platform_operations (
			operation_id, kind, binding_ref, pre_generation, target_generation, request_fingerprint,
			execution_token, lease_expires_at, state
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, "op-invalid", "OPERATION_KIND_BIND_CREDENTIAL", "binding-a", 1, 3, "fingerprint", "token", time.Now().UTC().Add(time.Minute), "pending")
	var postgresError *pgconn.PgError
	require.ErrorAs(t, err, &postgresError)
	require.Equal(t, "23514", postgresError.Code)
	require.Equal(t, "valid_operation_generation", postgresError.ConstraintName)
}

func TestV2BaselineRequiresPrimaryProfileOperationsToKeepCredentialGeneration(t *testing.T) {
	stack := newEmptyIntegrationStack(t)
	requireMigrationsApplied(t, stack.SQLDB)
	insert := func(operationID string, targetGeneration uint64) error {
		_, err := stack.SQLDB.Exec(`
			INSERT INTO platform_operations (
				operation_id, kind, binding_ref, pre_generation, target_generation, request_fingerprint,
				execution_token, lease_expires_at, state
			) VALUES ($1, 'OPERATION_KIND_SET_PRIMARY_PROFILE', 'binding-a', 1, $2, 'fingerprint', 'token', $3, 'pending')
		`, operationID, targetGeneration, time.Now().UTC().Add(time.Minute))
		return err
	}

	require.NoError(t, insert("op-primary-valid", 1))
	err := insert("op-primary-invalid", 2)
	var postgresError *pgconn.PgError
	require.ErrorAs(t, err, &postgresError)
	require.Equal(t, "23514", postgresError.Code)
	require.Equal(t, "valid_operation_generation", postgresError.ConstraintName)
}

func requireMigrationsApplied(t *testing.T, db *sql.DB) {
	t.Helper()
	pattern := filepath.Join(repoRoot(t), "initialize", "migrate", "sql", "*.up.sql")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob migration files: %v", err)
	}
	sort.Strings(files)
	for _, path := range files {
		if err := execMigrationFile(db, path); err != nil {
			t.Fatalf("apply migration %q: %v", path, err)
		}
	}
}

func insertCredential(t *testing.T, db *sql.DB, bindingRef string, accountKey string, accountID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO credential_records (
			binding_ref, account_key, generation, platform, account_id, region,
			credential_blob, credential_version, status
		) VALUES ($1, $2, 1, 'mihomo', $3, 'cn_gf01', '{}', 'v1', 'active')
	`, bindingRef, accountKey, accountID)
	if err != nil {
		t.Fatalf("insert credential: %v", err)
	}
}
