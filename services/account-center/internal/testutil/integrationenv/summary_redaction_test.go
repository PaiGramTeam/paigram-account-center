package integrationenv

import (
	"strings"
	"testing"
)

func TestSummaryLinesNeverLeaksCredentials(t *testing.T) {
	databaseSecret := "database-secret-NEVER-LEAK"
	redisSecret := "redis-secret-NEVER-LEAK"
	env := Env{
		RepoRoot:         "/tmp/repo",
		EnvFilePath:      "/tmp/repo/.env.integration.local",
		EnvFileLoaded:    true,
		GoWork:           "off",
		DatabaseDSN:      "postgres://user:" + databaseSecret + "@127.0.0.1:5432/paigram_test",
		RedisAddr:        "127.0.0.1:6379",
		RedisPassword:    redisSecret,
		RedisPrefix:      "itest",
		HasDatabaseDSN:   true,
		HasRedisPassword: true,
	}
	for _, line := range env.SummaryLines("doctor", true) {
		if strings.Contains(line, databaseSecret) || strings.Contains(line, redisSecret) {
			t.Errorf("SummaryLines leaked a credential in line: %q", line)
		}
	}
}

func TestCredentialTagReturnsLiterals(t *testing.T) {
	if got := credentialTag(false); got != "<empty>" {
		t.Errorf("credentialTag(false) = %q, want <empty>", got)
	}
	if got := credentialTag(true); got != "<redacted>" {
		t.Errorf("credentialTag(true) = %q, want <redacted>", got)
	}
}
