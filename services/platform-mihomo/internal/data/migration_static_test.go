package data

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBindingFirstMigrationDoesNotDeleteRows(t *testing.T) {
	migration := readMigrationForStaticTest(t, "000006_binding_first_devices_profiles_and_grant_invalidations.up.sql")
	normalized := normalizeSQLForStaticTest(migration)

	require.Regexp(t, regexp.MustCompile(`(?is)SIGNAL\s+SQLSTATE\s+'45000'[^;]*DEVICE_RECORDS`), migration)
	require.Regexp(t, regexp.MustCompile(`(?is)SIGNAL\s+SQLSTATE\s+'45000'[^;]*ACCOUNT_PROFILES`), migration)
	assertNoDestructiveTableMutation(t, normalized, "DEVICE_RECORDS")
	assertNoDestructiveTableMutation(t, normalized, "ACCOUNT_PROFILES")
}

func TestBindingFirstMigrationPrechecksProfileDuplicatesBeforeDDL(t *testing.T) {
	migration := readMigrationForStaticTest(t, "000006_binding_first_devices_profiles_and_grant_invalidations.up.sql")
	normalized := normalizeSQLForStaticTest(migration)

	firstAlter := strings.Index(normalized, "ALTER TABLE")
	require.NotEqual(t, -1, firstAlter)

	assertBeforeFirstAlter(t, normalized, firstAlter, "DUPLICATE ACCOUNT_PROFILES ROWS FOR BINDING_ID, PLAYER_ID, REGION")
}

func TestBindingFirstMigrationPrechecksDeviceBackfillBeforeDDL(t *testing.T) {
	migration := readMigrationForStaticTest(t, "000006_binding_first_devices_profiles_and_grant_invalidations.up.sql")
	normalized := normalizeSQLForStaticTest(migration)

	firstAlter := strings.Index(normalized, "ALTER TABLE")
	require.NotEqual(t, -1, firstAlter)

	assertBeforeFirstAlter(t, normalized, firstAlter, "DEVICE_RECORDS ROWS CANNOT BE BACKFILLED FROM CREDENTIAL_RECORDS")
}

func TestBindingIDBackfillMigrationPrechecksLegacyAccountIDsBeforeDDL(t *testing.T) {
	migration := readMigrationForStaticTest(t, "000005_add_binding_id_to_credentials_and_profiles.up.sql")
	normalized := normalizeSQLForStaticTest(migration)

	firstAlter := strings.Index(normalized, "ALTER TABLE")
	require.NotEqual(t, -1, firstAlter)

	assertBeforeFirstAlter(t, normalized, firstAlter, "MALFORMED CREDENTIAL_RECORDS PLATFORM_ACCOUNT_ID VALUES")
	assertBeforeFirstAlter(t, normalized, firstAlter, "DUPLICATE PARSED CREDENTIAL_RECORDS BINDING_ID VALUES")
	assertBeforeFirstAlter(t, normalized, firstAlter, "MALFORMED ACCOUNT_PROFILES PLATFORM_ACCOUNT_ID VALUES")
	require.Less(t, strings.Index(normalized, "DUPLICATE_PARSED_CREDENTIAL_BINDING_IDS"), firstAlter)
	require.NotContains(t, normalized, "DUPLICATE_PARSED_PROFILE_BINDING_IDS")
	require.NotContains(t, normalized, "DUPLICATE PARSED ACCOUNT_PROFILES BINDING_ID VALUES")
	require.Contains(t, normalized, "UNIQ_CREDENTIAL_BINDING_ID")
}

func TestRuntimeArtifactMigrationPrechecksHazardsBeforeDDL(t *testing.T) {
	migration := readMigrationForStaticTest(t, "000007_binding_first_runtime_artifacts_and_primary_profile.up.sql")
	normalized := normalizeSQLForStaticTest(migration)

	firstAlter := strings.Index(normalized, "ALTER TABLE")
	require.NotEqual(t, -1, firstAlter)

	assertBeforeFirstAlter(t, normalized, firstAlter, "MULTIPLE DEFAULT ACCOUNT_PROFILES ROWS FOR BINDING_ID")
	assertBeforeFirstAlter(t, normalized, firstAlter, "RUNTIME_ARTIFACTS ROWS WITHOUT CREDENTIAL BINDING_ID MAPPING")
	assertBeforeFirstAlter(t, normalized, firstAlter, "DUPLICATE RUNTIME_ARTIFACTS ROWS FOR BINDING_ID, ARTIFACT_TYPE, SCOPE_KEY")
}

func TestRuntimeArtifactMigrationRollbackPrechecksPlatformUniquenessBeforeDDL(t *testing.T) {
	migration := readMigrationForStaticTest(t, "000007_binding_first_runtime_artifacts_and_primary_profile.down.sql")
	normalized := normalizeSQLForStaticTest(migration)

	firstAlter := strings.Index(normalized, "ALTER TABLE")
	require.NotEqual(t, -1, firstAlter)

	assertBeforeFirstAlter(t, normalized, firstAlter, "RUNTIME_ARTIFACTS ROWS WOULD VIOLATE PLATFORM_ACCOUNT_ID UNIQUENESS")
}

func TestDestructiveTableMutationPatternDetectsMultiTableDrop(t *testing.T) {
	profilePattern := destructiveTableMutationPattern("ACCOUNT_PROFILES")
	devicePattern := destructiveTableMutationPattern("DEVICE_RECORDS")

	require.Regexp(t, profilePattern, "DROP TABLE OTHER_TABLE, ACCOUNT_PROFILES")
	require.Regexp(t, devicePattern, "DROP TEMPORARY TABLE IF EXISTS DEVICE_RECORDS, OTHER_TABLE")
	require.NotRegexp(t, profilePattern, "DROP TABLE ACCOUNT_PROFILE_SNAPSHOTS")
}

func readMigrationForStaticTest(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join("..", "..", "initialize", "migrate", "sql", name)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.ToUpper(string(contents))
}

func normalizeSQLForStaticTest(sql string) string {
	normalized := strings.NewReplacer("`", "", "\"", "").Replace(sql)
	return strings.Join(strings.Fields(normalized), " ")
}

func assertNoDestructiveTableMutation(t *testing.T, sql, table string) {
	t.Helper()

	identifier := regexp.QuoteMeta(table)
	require.NotRegexp(t, regexp.MustCompile(`(?i)\bDELETE\b[^;]*\b`+identifier+`\b`), sql)
	require.NotRegexp(t, regexp.MustCompile(`(?i)\bTRUNCATE\b\s+(?:\bTABLE\b\s+)?\b`+identifier+`\b`), sql)
	require.NotRegexp(t, destructiveTableMutationPattern(table), sql)
}

func assertBeforeFirstAlter(t *testing.T, sql string, firstAlter int, required string) {
	t.Helper()

	index := strings.Index(sql, required)
	require.NotEqual(t, -1, index, "missing precheck %q", required)
	require.Less(t, index, firstAlter, "precheck %q must run before first ALTER TABLE", required)
}

func destructiveTableMutationPattern(table string) *regexp.Regexp {
	identifier := regexp.QuoteMeta(table)
	return regexp.MustCompile(`(?i)\bDROP\b\s+(?:\bTEMPORARY\b\s+)?\bTABLE\b\s+(?:\bIF\b\s+\bEXISTS\b\s+)?[^;]*\b` + identifier + `\b`)
}
