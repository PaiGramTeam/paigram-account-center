package integrationenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadUsesPAITestVariablesInsteadOfLegacyDSN(t *testing.T) {
	repoRoot := t.TempDir()
	err := os.WriteFile(filepath.Join(repoRoot, ".env.integration.local"), []byte(strings.Join([]string{
		"PAI_TEST_DATABASE_ADDR=127.0.0.1:3306",
		"PAI_TEST_DATABASE_USERNAME=root",
		"PAI_TEST_DATABASE_PASSWORD=root",
		"PAI_TEST_DATABASE_DBNAME=platform_mihomo_test",
	}, "\n")), 0o600)
	require.NoError(t, err)

	t.Setenv("TEST_DATABASE_SOURCE", "root:root@tcp(127.0.0.1:3306)/legacy?charset=utf8mb4")

	env, err := Load(repoRoot)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:3306", env.DatabaseAddr)
	require.Equal(t, "platform_mihomo_test", env.DatabaseBaseName)
	require.NotContains(t, fmt.Sprintf("%+v", env), "legacy")
}

func TestLoadPrefersShellValuesOverDotEnvAndAppliesDefaults(t *testing.T) {
	repoRoot := t.TempDir()
	err := os.WriteFile(filepath.Join(repoRoot, ".env.integration.local"), []byte(strings.Join([]string{
		"PAI_TEST_DATABASE_ADDR=file-mysql:3306",
		"PAI_TEST_DATABASE_USERNAME=file-user",
		"PAI_TEST_DATABASE_PASSWORD=file-pass",
		"PAI_TEST_DATABASE_DBNAME=file_base",
		"PAI_TEST_REDIS_ADDR=file-redis:6379",
	}, "\n")), 0o600)
	require.NoError(t, err)

	t.Setenv("PAI_TEST_DATABASE_ADDR", "shell-mysql:3306")

	env, err := Load(repoRoot)
	require.NoError(t, err)
	require.Equal(t, "shell-mysql:3306", env.DatabaseAddr)
	require.Equal(t, SourceShell, env.Sources["PAI_TEST_DATABASE_ADDR"])
	require.Equal(t, "file-user", env.DatabaseUsername)
	require.Equal(t, SourceFile, env.Sources["PAI_TEST_DATABASE_USERNAME"])
	require.Equal(t, "charset=utf8mb4&parseTime=True&loc=UTC&multiStatements=true", env.DatabaseConfig)
	require.Equal(t, SourceDefault, env.Sources["PAI_TEST_DATABASE_CONFIG"])
	require.Equal(t, "itest", env.RedisPrefix)
	require.Equal(t, SourceDefault, env.Sources["PAI_TEST_REDIS_PREFIX"])
	require.Equal(t, 0, env.RedisDB)
	require.Equal(t, SourceDefault, env.Sources["PAI_TEST_REDIS_DB"])
}

func TestSummaryIncludesGeneratedResourcesAndRedactsSecrets(t *testing.T) {
	env := Env{
		DatabaseAddr:     "127.0.0.1:3306",
		DatabaseUsername: "root",
		DatabasePassword: "super-secret",
		DatabaseBaseName: "platform_mihomo_test",
		DatabaseConfig:   "charset=utf8mb4&parseTime=True&loc=UTC&multiStatements=true",
		RedisAddr:        "127.0.0.1:6379",
		RedisPassword:    "redis-secret",
		RedisDB:          2,
		RedisPrefix:      "itest",
		EnvFile:          filepath.Join("repo", ".env.integration.local"),
		Sources: map[string]Source{
			"PAI_TEST_DATABASE_ADDR":     SourceShell,
			"PAI_TEST_DATABASE_USERNAME": SourceFile,
			"PAI_TEST_DATABASE_PASSWORD": SourceFile,
			"PAI_TEST_DATABASE_DBNAME":   SourceFile,
			"PAI_TEST_DATABASE_CONFIG":   SourceDefault,
			"PAI_TEST_REDIS_ADDR":        SourceFile,
			"PAI_TEST_REDIS_PASSWORD":    SourceFile,
			"PAI_TEST_REDIS_DB":          SourceDefault,
			"PAI_TEST_REDIS_PREFIX":      SourceDefault,
		},
	}

	summary := strings.Join(env.Summary("platform_mihomo_test_doctor_a1b2c3d4", "itest:doctor:a1b2c3d4", "off", false), "\n")
	require.Contains(t, summary, "database.name=platform_mihomo_test_doctor_a1b2c3d4")
	require.Contains(t, summary, "redis.prefix=itest:doctor:a1b2c3d4")
	require.Contains(t, summary, "redis.required=false")
	require.Contains(t, summary, "gowork=off")
	require.Contains(t, summary, "database.password=<redacted>")
	require.Contains(t, summary, "redis.password=<redacted>")
	require.NotContains(t, summary, "super-secret")
	require.NotContains(t, summary, "redis-secret")
}

func TestLoadRejectsInvalidRedisDBAndPreservesSource(t *testing.T) {
	repoRoot := t.TempDir()
	err := os.WriteFile(filepath.Join(repoRoot, ".env.integration.local"), []byte(strings.Join([]string{
		"PAI_TEST_DATABASE_ADDR=127.0.0.1:3306",
		"PAI_TEST_DATABASE_USERNAME=root",
		"PAI_TEST_DATABASE_PASSWORD=root",
		"PAI_TEST_DATABASE_DBNAME=platform_mihomo_test",
		"PAI_TEST_REDIS_DB=not-a-number",
	}, "\n")), 0o600)
	require.NoError(t, err)

	env, err := Load(repoRoot)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PAI_TEST_REDIS_DB")
	require.Equal(t, SourceFile, env.Sources["PAI_TEST_REDIS_DB"])
}
