//go:build integration

package integration

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestMigrationsCreateCoreTables(t *testing.T) {
	stack := newIntegrationStack(t)
	t.Cleanup(stack.cleanup)

	requireMigrationsApplied(t, stack.SQLDB)
	requireTableExists(t, stack.SQLDB, stack.DatabaseCfg.Dbname, "credential_records")
	requireTableExists(t, stack.SQLDB, stack.DatabaseCfg.Dbname, "device_records")
	requireTableExists(t, stack.SQLDB, stack.DatabaseCfg.Dbname, "account_profiles")
	requireTableExists(t, stack.SQLDB, stack.DatabaseCfg.Dbname, "runtime_artifacts")
}

func TestBindingMigrationRejectsUnknownLegacyPlatformAccountIDs(t *testing.T) {
	stack := newIntegrationStack(t)
	t.Cleanup(stack.cleanup)

	applyMigrationFile(t, stack.SQLDB, migrationPath(t, "000001_create_credential_records.up.sql"))
	applyMigrationFile(t, stack.SQLDB, migrationPath(t, "000003_create_account_profiles.up.sql"))

	_, err := stack.SQLDB.Exec(`
		INSERT INTO credential_records (
			platform_account_id, platform, account_id, region, credential_blob, credential_version, status
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "legacy_10001", "mihomo", "10001", "cn_gf01", "{}", "v1", "active")
	if err != nil {
		t.Fatalf("insert legacy credential row: %v", err)
	}

	_, err = stack.SQLDB.Exec(`
		INSERT INTO account_profiles (
			platform_account_id, game_biz, region, player_id, nickname, level, is_default
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "legacy_10001", "hk4e_cn", "cn_gf01", "1008611", "Traveler", 1, true)
	if err != nil {
		t.Fatalf("insert legacy profile row: %v", err)
	}

	err = execMigrationFile(stack.SQLDB, migrationPath(t, "000005_add_binding_id_to_credentials_and_profiles.up.sql"))
	if err == nil {
		t.Fatal("expected binding migration to fail for unknown legacy platform_account_id")
	}
}

func TestRuntimeArtifactMigrationRejectsDuplicateDefaultProfiles(t *testing.T) {
	stack := newIntegrationStack(t)
	t.Cleanup(stack.cleanup)
	applyMigrations(t, stack.SQLDB,
		"000001_create_credential_records.up.sql",
		"000002_create_device_records.up.sql",
		"000003_create_account_profiles.up.sql",
		"000004_create_runtime_artifacts.up.sql",
	)

	insertLegacyCredential(t, stack.SQLDB, "binding_42_10001", "10001")
	insertLegacyProfile(t, stack.SQLDB, "binding_42_10001", "1008611", true)
	insertLegacyProfile(t, stack.SQLDB, "binding_42_10001", "1008622", true)
	applyMigrations(t, stack.SQLDB,
		"000005_add_binding_id_to_credentials_and_profiles.up.sql",
		"000006_binding_first_devices_profiles_and_grant_invalidations.up.sql",
	)

	err := execMigrationFile(stack.SQLDB, migrationPath(t, "000007_binding_first_runtime_artifacts_and_primary_profile.up.sql"))
	if err == nil {
		t.Fatal("expected runtime artifact migration to fail for duplicate default profiles")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "multiple default account_profiles rows") {
		t.Fatalf("expected duplicate default profile precheck error, got: %v", err)
	}
}

func TestRuntimeArtifactMigrationRejectsUnmappableArtifacts(t *testing.T) {
	stack := newIntegrationStack(t)
	t.Cleanup(stack.cleanup)
	applyMigrations(t, stack.SQLDB,
		"000001_create_credential_records.up.sql",
		"000002_create_device_records.up.sql",
		"000003_create_account_profiles.up.sql",
		"000004_create_runtime_artifacts.up.sql",
	)

	insertLegacyCredential(t, stack.SQLDB, "binding_42_10001", "10001")
	insertLegacyProfile(t, stack.SQLDB, "binding_42_10001", "1008611", true)
	insertLegacyRuntimeArtifact(t, stack.SQLDB, "binding_99_missing", "authkey", "orphan-authkey", "1008611")
	applyMigrations(t, stack.SQLDB,
		"000005_add_binding_id_to_credentials_and_profiles.up.sql",
		"000006_binding_first_devices_profiles_and_grant_invalidations.up.sql",
	)

	err := execMigrationFile(stack.SQLDB, migrationPath(t, "000007_binding_first_runtime_artifacts_and_primary_profile.up.sql"))
	if err == nil {
		t.Fatal("expected runtime artifact migration to fail for unmappable runtime artifacts")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "runtime_artifacts rows without credential binding_id mapping") {
		t.Fatalf("expected unmappable runtime artifact precheck error, got: %v", err)
	}
}

func TestRuntimeArtifactMigrationRejectsDuplicateBindingArtifacts(t *testing.T) {
	stack := newIntegrationStack(t)
	t.Cleanup(stack.cleanup)
	applyMigrations(t, stack.SQLDB,
		"000001_create_credential_records.up.sql",
		"000002_create_device_records.up.sql",
		"000003_create_account_profiles.up.sql",
		"000004_create_runtime_artifacts.up.sql",
	)

	insertLegacyCredential(t, stack.SQLDB, "binding_42_10001", "10001")
	insertLegacyProfile(t, stack.SQLDB, "binding_42_10001", "1008611", true)
	applyMigrations(t, stack.SQLDB,
		"000005_add_binding_id_to_credentials_and_profiles.up.sql",
		"000006_binding_first_devices_profiles_and_grant_invalidations.up.sql",
	)
	allowDuplicateCredentialBindingIDs(t, stack.SQLDB)
	insertCredentialWithBindingID(t, stack.SQLDB, 42, "binding_42_20002", "20002")
	insertLegacyRuntimeArtifact(t, stack.SQLDB, "binding_42_10001", "authkey", "first-authkey", "1008611")
	insertLegacyRuntimeArtifact(t, stack.SQLDB, "binding_42_20002", "authkey", "second-authkey", "1008611")

	err := execMigrationFile(stack.SQLDB, migrationPath(t, "000007_binding_first_runtime_artifacts_and_primary_profile.up.sql"))
	if err == nil {
		t.Fatal("expected runtime artifact migration to fail for duplicate binding artifacts")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "duplicate runtime_artifacts rows for binding_id") {
		t.Fatalf("expected duplicate binding runtime artifact precheck error, got: %v", err)
	}
}

func TestRuntimeArtifactRollbackRejectsDuplicatePlatformArtifacts(t *testing.T) {
	stack := newIntegrationStack(t)
	t.Cleanup(stack.cleanup)
	requireMigrationsApplied(t, stack.SQLDB)

	insertRuntimeArtifactWithBindingID(t, stack.SQLDB, 42, "binding_42_10001", "authkey", "first-authkey", "1008611")
	insertRuntimeArtifactWithBindingID(t, stack.SQLDB, 43, "binding_42_10001", "authkey", "second-authkey", "1008611")

	err := execMigrationFile(stack.SQLDB, migrationPath(t, "000007_binding_first_runtime_artifacts_and_primary_profile.down.sql"))
	if err == nil {
		t.Fatal("expected runtime artifact rollback to fail for duplicate platform artifacts")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "runtime_artifacts rows would violate platform_account_id uniqueness") {
		t.Fatalf("expected duplicate platform runtime artifact rollback precheck error, got: %v", err)
	}
}

func requireMigrationsApplied(t *testing.T, db *sql.DB) {
	t.Helper()

	pattern := filepath.Join(repoRoot(t), "initialize", "migrate", "sql", "*.up.sql")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob migration files: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no migration files found for pattern %q", pattern)
	}
	sort.Strings(files)

	for _, path := range files {
		if err := execMigrationFile(db, path); err != nil {
			t.Fatalf("apply migration %q: %v", path, err)
		}
	}
}

func migrationPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "initialize", "migrate", "sql", name)
}

func applyMigrationFile(t *testing.T, db *sql.DB, path string) {
	t.Helper()
	if err := execMigrationFile(db, path); err != nil {
		t.Fatalf("apply migration %q: %v", path, err)
	}
}

func applyMigrations(t *testing.T, db *sql.DB, names ...string) {
	t.Helper()
	for _, name := range names {
		applyMigrationFile(t, db, migrationPath(t, name))
	}
}

func insertLegacyCredential(t *testing.T, db *sql.DB, platformAccountID string, accountID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO credential_records (
			platform_account_id, platform, account_id, region, credential_blob, credential_version, status
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, platformAccountID, "mihomo", accountID, "cn_gf01", "{}", "v1", "active")
	if err != nil {
		t.Fatalf("insert legacy credential row: %v", err)
	}
}

func insertLegacyProfile(t *testing.T, db *sql.DB, platformAccountID string, playerID string, isDefault bool) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO account_profiles (
			platform_account_id, game_biz, region, player_id, nickname, level, is_default
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, platformAccountID, "hk4e_cn", "cn_gf01", playerID, "Traveler", 1, isDefault)
	if err != nil {
		t.Fatalf("insert legacy profile row: %v", err)
	}
}

func insertLegacyRuntimeArtifact(t *testing.T, db *sql.DB, platformAccountID string, artifactType string, artifactValue string, scopeKey string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO runtime_artifacts (
			platform_account_id, artifact_type, artifact_value, scope_key, expires_at
		) VALUES (?, ?, ?, ?, DATE_ADD(UTC_TIMESTAMP(3), INTERVAL 1 HOUR))
	`, platformAccountID, artifactType, artifactValue, scopeKey)
	if err != nil {
		t.Fatalf("insert legacy runtime artifact row: %v", err)
	}
}

func allowDuplicateCredentialBindingIDs(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`ALTER TABLE credential_records DROP INDEX uniq_credential_binding_id`); err != nil {
		t.Fatalf("drop credential binding uniqueness: %v", err)
	}
}

func insertCredentialWithBindingID(t *testing.T, db *sql.DB, bindingID uint64, platformAccountID string, accountID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO credential_records (
			binding_id, platform_account_id, platform, account_id, region, credential_blob, credential_version, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, bindingID, platformAccountID, "mihomo", accountID, "cn_gf01", "{}", "v1", "active")
	if err != nil {
		t.Fatalf("insert credential row with binding_id: %v", err)
	}
}

func insertRuntimeArtifactWithBindingID(t *testing.T, db *sql.DB, bindingID uint64, platformAccountID string, artifactType string, artifactValue string, scopeKey string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO runtime_artifacts (
			binding_id, platform_account_id, artifact_type, artifact_value, scope_key, expires_at
		) VALUES (?, ?, ?, ?, ?, DATE_ADD(UTC_TIMESTAMP(3), INTERVAL 1 HOUR))
	`, bindingID, platformAccountID, artifactType, artifactValue, scopeKey)
	if err != nil {
		t.Fatalf("insert runtime artifact row with binding_id: %v", err)
	}
}

func execMigrationFile(db *sql.DB, path string) error {
	statement, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration %q: %w", path, err)
	}
	if _, err := db.Exec(string(statement)); err != nil {
		return err
	}
	return nil
}

func requireTableExists(t *testing.T, db *sql.DB, schema string, table string) {
	t.Helper()

	const query = `
		SELECT 1
		FROM information_schema.tables
		WHERE table_schema = ? AND table_name = ?
		LIMIT 1
	`

	var exists int
	err := db.QueryRow(query, schema, table).Scan(&exists)
	if err == nil {
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("table %q does not exist in schema %q", table, schema)
	}
	t.Fatalf("query table %q existence: %v", table, err)
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file path")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), ".."))
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("stat repo root %q: %v", root, err)
	}

	return root
}
