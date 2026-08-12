package data

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBaselineMigrationUsesStableReferencesAndOperationConstraints(t *testing.T) {
	migration := readBaselineMigration(t)

	for _, fragment := range []string{
		"binding_ref varchar(64) not null",
		"account_key varchar(64) not null",
		"profile_ref varchar(64) not null",
		"device_ref varchar(64) not null",
		"create table platform_operations",
		"request_fingerprint varchar(64) not null",
		"target_generation = pre_generation + 1",
		"create table authorization_fences",
		"foreign key (binding_ref, account_key)",
	} {
		require.Contains(t, migration, fragment)
	}
	require.NotContains(t, migration, "binding_id")
	require.NotContains(t, migration, "platform_account_id")
}

func TestBaselineMigrationHasSymmetricDownMigration(t *testing.T) {
	down := readMigrationFile(t, "000001_init_schema.down.sql")
	for _, table := range []string{
		"platform_operations",
		"authorization_fences",
		"consumer_grant_invalidations",
		"artifact_revocation_intents",
		"runtime_artifacts",
		"account_profiles",
		"device_records",
		"credential_records",
	} {
		require.Contains(t, down, "drop table if exists "+table)
	}
}

func readBaselineMigration(t *testing.T) string {
	t.Helper()
	return readMigrationFile(t, "000001_init_schema.up.sql")
}

func readMigrationFile(t *testing.T, name string) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "initialize", "migrate", "sql", name)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.ToLower(string(content))
}
