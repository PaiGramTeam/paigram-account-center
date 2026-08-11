package integrationenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadUsesPostgreSQLDSNFromEnvFile(t *testing.T) {
	repoRoot := t.TempDir()
	dsn := "postgres://platform:file-secret@127.0.0.1:5432/platform_mihomo_test?sslmode=disable"
	err := os.WriteFile(filepath.Join(repoRoot, ".env.integration.local"), []byte(strings.Join([]string{
		"PAI_TEST_DATABASE_DSN=" + dsn,
		"PAI_TEST_REDIS_ADDR=127.0.0.1:6379",
	}, "\n")), 0o600)
	require.NoError(t, err)

	env, err := Load(repoRoot)
	require.NoError(t, err)
	require.Equal(t, dsn, env.DatabaseDSN)
	require.Equal(t, SourceFile, env.Sources["PAI_TEST_DATABASE_DSN"])
	require.Equal(t, "itest", env.RedisPrefix)
	require.Equal(t, SourceDefault, env.Sources["PAI_TEST_REDIS_PREFIX"])
}

func TestLoadPrefersShellPostgreSQLDSN(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".env.integration.local"), []byte(
		"PAI_TEST_DATABASE_DSN=postgres://file:file-secret@file-host:5432/file_db\n",
	), 0o600))
	shellDSN := "postgres://shell:shell-secret@shell-host:5432/shell_db?sslmode=disable"
	t.Setenv("PAI_TEST_DATABASE_DSN", shellDSN)

	env, err := Load(repoRoot)
	require.NoError(t, err)
	require.Equal(t, shellDSN, env.DatabaseDSN)
	require.Equal(t, SourceShell, env.Sources["PAI_TEST_DATABASE_DSN"])
}

func TestDatabaseNameReadsPostgreSQLDSN(t *testing.T) {
	env := Env{DatabaseDSN: "postgres://platform:secret@127.0.0.1:5432/platform_mihomo_test?sslmode=disable"}

	name, err := env.DatabaseName()
	require.NoError(t, err)
	require.Equal(t, "platform_mihomo_test", name)
}

func TestSummaryIncludesGeneratedResourcesAndRedactsSecrets(t *testing.T) {
	env := Env{
		DatabaseDSN:   "postgres://platform:super-secret@127.0.0.1:5432/platform_mihomo_test?sslmode=disable",
		RedisAddr:     "127.0.0.1:6379",
		RedisPassword: "redis-secret",
		RedisDB:       2,
		RedisPrefix:   "itest",
		EnvFile:       filepath.Join("repo", ".env.integration.local"),
		Sources: map[string]Source{
			"PAI_TEST_DATABASE_DSN":   SourceShell,
			"PAI_TEST_REDIS_ADDR":     SourceFile,
			"PAI_TEST_REDIS_PASSWORD": SourceFile,
			"PAI_TEST_REDIS_DB":       SourceDefault,
			"PAI_TEST_REDIS_PREFIX":   SourceDefault,
		},
	}

	summary := strings.Join(env.Summary("platform_mihomo_test_doctor_a1b2c3d4", "itest:doctor:a1b2c3d4", "off", false), "\n")
	require.Contains(t, summary, "postgres.dsn=<redacted> (source=shell)")
	require.Contains(t, summary, "database.name=platform_mihomo_test_doctor_a1b2c3d4")
	require.Contains(t, summary, "redis.prefix=itest:doctor:a1b2c3d4")
	require.Contains(t, summary, "redis.required=false")
	require.Contains(t, summary, "gowork=off")
	require.Contains(t, summary, "redis.password=<redacted>")
	require.NotContains(t, summary, "super-secret")
	require.NotContains(t, summary, "redis-secret")
}

func TestLoadRejectsInvalidRedisDBAndPreservesSource(t *testing.T) {
	repoRoot := t.TempDir()
	err := os.WriteFile(filepath.Join(repoRoot, ".env.integration.local"), []byte(strings.Join([]string{
		"PAI_TEST_DATABASE_DSN=postgres://platform:password@127.0.0.1:5432/platform_mihomo_test?sslmode=disable",
		"PAI_TEST_REDIS_DB=not-a-number",
	}, "\n")), 0o600)
	require.NoError(t, err)

	env, err := Load(repoRoot)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PAI_TEST_REDIS_DB")
	require.Equal(t, SourceFile, env.Sources["PAI_TEST_REDIS_DB"])
}
