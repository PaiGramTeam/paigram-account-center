package integrationenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadUsesEnvFileDefaultsAndTracksSources(t *testing.T) {
	dsn := "postgres://test_user:file-secret@127.0.0.1:5432/acctest?sslmode=disable"
	repoRoot := newTempRepoRoot(t, "PAI_TEST_DATABASE_DSN="+dsn+"\nPAI_TEST_REDIS_ADDR=127.0.0.1:6379\n")
	env, err := Load(LoadOptions{WorkingDir: filepath.Join(repoRoot, "integration"), LookupEnv: emptyLookupEnv})
	require.NoError(t, err)

	require.Equal(t, repoRoot, env.RepoRoot)
	require.True(t, env.EnvFileLoaded)
	require.Equal(t, dsn, env.DatabaseDSN)
	require.Equal(t, SourceFile, env.Sources.DatabaseDSN)
	require.Equal(t, "127.0.0.1:6379", env.RedisAddr)
	require.Equal(t, defaultRedisPrefix, env.RedisPrefix)
	require.Empty(t, env.MissingRequired())

	lines := strings.Join(env.SummaryLines("doctor", true), "\n")
	require.Contains(t, lines, "postgres.dsn=<redacted> (file)")
	require.Contains(t, lines, "sample.postgres.database=t_acctest_")
	require.Contains(t, lines, "redis.required=true")
	require.NotContains(t, lines, "file-secret")
}

func TestLoadShellEnvOverridesFileValues(t *testing.T) {
	repoRoot := newTempRepoRoot(t, "PAI_TEST_DATABASE_DSN=postgres://file:file-secret@file-host:5432/file-db\nPAI_TEST_REDIS_ADDR=file-redis:6379\n")
	shellDSN := "postgres://shell:shell-secret@shell-host:5432/shell-db?sslmode=disable"
	env, err := Load(LoadOptions{
		WorkingDir: filepath.Join(repoRoot, "integration"),
		LookupEnv: mapLookupEnv(map[string]string{
			"PAI_TEST_DATABASE_DSN":   shellDSN,
			"PAI_TEST_REDIS_ADDR":     "shell-redis:6379",
			"PAI_TEST_REDIS_PASSWORD": "shell-redis-secret",
			"PAI_TEST_REDIS_DB":       "5",
			"PAI_TEST_REDIS_PREFIX":   "shell-prefix",
			"GOWORK":                  filepath.Join(repoRoot, "go.work"),
		}),
	})
	require.NoError(t, err)
	require.Equal(t, shellDSN, env.DatabaseDSN)
	require.Equal(t, SourceShell, env.Sources.DatabaseDSN)
	require.Equal(t, 5, env.RedisDB)
	require.Equal(t, SourceShell, env.Sources.RedisDB)

	lines := strings.Join(env.SummaryLines("doctor", true), "\n")
	require.Contains(t, lines, "postgres.dsn=<redacted> (shell)")
	require.Contains(t, lines, "redis.db=5 (shell)")
	require.NotContains(t, lines, "shell-secret")
	require.NotContains(t, lines, "shell-redis-secret")
}

func TestLoadReportsMissingRequiredFields(t *testing.T) {
	repoRoot := newTempRepoRoot(t, "")
	env, err := Load(LoadOptions{WorkingDir: filepath.Join(repoRoot, "integration"), LookupEnv: emptyLookupEnv})
	require.NoError(t, err)
	require.Equal(t, []string{"PAI_TEST_DATABASE_DSN", "PAI_TEST_REDIS_ADDR"}, env.MissingRequired())
}

func newTempRepoRoot(t *testing.T, envFile string) string {
	t.Helper()
	repoRoot := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repoRoot, "integration"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module paigram\n\ngo 1.25.7\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, envFileName), []byte(envFile), 0o644))
	return repoRoot
}

func emptyLookupEnv(string) (string, bool) { return "", false }

func mapLookupEnv(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
