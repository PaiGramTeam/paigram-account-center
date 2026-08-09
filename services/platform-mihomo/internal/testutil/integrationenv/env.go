package integrationenv

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

const defaultDatabaseConfig = "charset=utf8mb4&parseTime=True&loc=UTC&multiStatements=true"

type Source string

const (
	SourceShell   Source = "shell"
	SourceFile    Source = "file"
	SourceDefault Source = "default"
)

type Env struct {
	DatabaseAddr     string
	DatabaseUsername string
	DatabasePassword string
	DatabaseBaseName string
	DatabaseConfig   string
	RedisAddr        string
	RedisPassword    string
	RedisDB          int
	RedisPrefix      string
	EnvFile          string
	Sources          map[string]Source
}

func Load(repoRoot string) (Env, error) {
	envFile := filepath.Join(repoRoot, ".env.integration.local")
	fileValues, err := readDotEnv(envFile)
	if err != nil {
		return Env{}, err
	}

	env := Env{EnvFile: envFile, Sources: map[string]Source{}}
	env.DatabaseAddr = pickString(env.Sources, "PAI_TEST_DATABASE_ADDR", fileValues)
	env.DatabaseUsername = pickString(env.Sources, "PAI_TEST_DATABASE_USERNAME", fileValues)
	env.DatabasePassword = pickString(env.Sources, "PAI_TEST_DATABASE_PASSWORD", fileValues)
	env.DatabaseBaseName = pickString(env.Sources, "PAI_TEST_DATABASE_DBNAME", fileValues)
	env.DatabaseConfig = pickStringDefault(env.Sources, "PAI_TEST_DATABASE_CONFIG", fileValues, defaultDatabaseConfig)
	env.RedisAddr = pickString(env.Sources, "PAI_TEST_REDIS_ADDR", fileValues)
	env.RedisPassword = pickString(env.Sources, "PAI_TEST_REDIS_PASSWORD", fileValues)
	redisDB, err := pickIntDefault(env.Sources, "PAI_TEST_REDIS_DB", fileValues, 0)
	if err != nil {
		return env, err
	}
	env.RedisDB = redisDB
	env.RedisPrefix = pickStringDefault(env.Sources, "PAI_TEST_REDIS_PREFIX", fileValues, "itest")

	return env, nil
}

func (e Env) Summary(generatedDBName, generatedRedisPrefix, gowork string, requireRedis bool) []string {
	if gowork == "" {
		gowork = "<unset>"
	}

	return []string{
		fmt.Sprintf("env.file=%s", valueOrUnset(e.EnvFile)),
		fmt.Sprintf("database.addr=%s (source=%s)", valueOrUnset(e.DatabaseAddr), sourceOrUnset(e.Sources, "PAI_TEST_DATABASE_ADDR")),
		fmt.Sprintf("database.username=%s (source=%s)", valueOrUnset(e.DatabaseUsername), sourceOrUnset(e.Sources, "PAI_TEST_DATABASE_USERNAME")),
		fmt.Sprintf("database.password=%s (source=%s)", redactSecret(e.DatabasePassword), sourceOrUnset(e.Sources, "PAI_TEST_DATABASE_PASSWORD")),
		fmt.Sprintf("database.base=%s (source=%s)", valueOrUnset(e.DatabaseBaseName), sourceOrUnset(e.Sources, "PAI_TEST_DATABASE_DBNAME")),
		fmt.Sprintf("database.name=%s", valueOrUnset(generatedDBName)),
		fmt.Sprintf("database.config=%s (source=%s)", valueOrUnset(e.DatabaseConfig), sourceOrUnset(e.Sources, "PAI_TEST_DATABASE_CONFIG")),
		fmt.Sprintf("redis.required=%t", requireRedis),
		fmt.Sprintf("redis.addr=%s (source=%s)", valueOrUnset(e.RedisAddr), sourceOrUnset(e.Sources, "PAI_TEST_REDIS_ADDR")),
		fmt.Sprintf("redis.password=%s (source=%s)", redactSecret(e.RedisPassword), sourceOrUnset(e.Sources, "PAI_TEST_REDIS_PASSWORD")),
		fmt.Sprintf("redis.db=%d (source=%s)", e.RedisDB, sourceOrUnset(e.Sources, "PAI_TEST_REDIS_DB")),
		fmt.Sprintf("redis.prefix=%s (source=%s)", valueOrUnset(generatedRedisPrefix), sourceOrUnset(e.Sources, "PAI_TEST_REDIS_PREFIX")),
		fmt.Sprintf("gowork=%s", gowork),
	}
}

func UniqueDatabaseName(baseName, testName string) string {
	base := sanitizeIdentifier(baseName, '_', "itest")
	name := sanitizeIdentifier(testName, '_', "test")
	suffix := randomSuffix()
	value := strings.Join([]string{base, name, suffix}, "_")
	if len(value) <= 64 {
		return value
	}

	maxBaseLen := 64 - len(name) - len(suffix) - 2
	if maxBaseLen < 8 {
		maxBaseLen = 8
	}
	if len(base) > maxBaseLen {
		base = strings.Trim(base[:maxBaseLen], "_")
		if base == "" {
			base = "itest"
		}
	}

	value = strings.Join([]string{base, name, suffix}, "_")
	if len(value) > 64 {
		value = value[len(value)-64:]
	}
	return strings.Trim(value, "_")
}

func UniqueRedisPrefix(basePrefix, testName string) string {
	base := sanitizeIdentifier(basePrefix, ':', "itest")
	name := sanitizeIdentifier(testName, ':', "test")
	return strings.Join([]string{base, name, randomSuffix()}, ":")
}

func CheckMySQL(env Env) error {
	if strings.TrimSpace(env.DatabaseAddr) == "" {
		return fmt.Errorf("PAI_TEST_DATABASE_ADDR is required")
	}
	if strings.TrimSpace(env.DatabaseUsername) == "" {
		return fmt.Errorf("PAI_TEST_DATABASE_USERNAME is required")
	}
	if strings.TrimSpace(env.DatabaseBaseName) == "" {
		return fmt.Errorf("PAI_TEST_DATABASE_DBNAME is required")
	}

	dsn := mysqlDSN(env, "")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}

	return nil
}

func CheckRedis(env Env, requireRedis bool) error {
	if strings.TrimSpace(env.RedisAddr) == "" {
		if requireRedis {
			return fmt.Errorf("PAI_TEST_REDIS_ADDR is required")
		}
		return nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     env.RedisAddr,
		Password: env.RedisPassword,
		DB:       env.RedisDB,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}

	return nil
}

func readDotEnv(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	values := map[string]string{}
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("parse %s: invalid line %q", path, rawLine)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("parse %s: empty key in line %q", path, rawLine)
		}

		if len(value) >= 2 {
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = strings.Trim(value, "\"")
			}
			if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
				value = strings.Trim(value, "'")
			}
		}

		values[key] = value
	}

	return values, nil
}

func pickString(sources map[string]Source, key string, fileValues map[string]string) string {
	if value, ok := os.LookupEnv(key); ok {
		sources[key] = SourceShell
		return value
	}
	if value, ok := fileValues[key]; ok {
		sources[key] = SourceFile
		return value
	}
	return ""
}

func pickStringDefault(sources map[string]Source, key string, fileValues map[string]string, fallback string) string {
	if value := pickString(sources, key, fileValues); value != "" || sources[key] != "" {
		return value
	}
	sources[key] = SourceDefault
	return fallback
}

func pickIntDefault(sources map[string]Source, key string, fileValues map[string]string, fallback int) (int, error) {
	if raw, ok := os.LookupEnv(key); ok {
		sources[key] = SourceShell
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fallback, fmt.Errorf("parse %s from %s: %w", key, SourceShell, err)
		}
		return value, nil
	}
	if raw, ok := fileValues[key]; ok {
		sources[key] = SourceFile
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fallback, fmt.Errorf("parse %s from %s: %w", key, SourceFile, err)
		}
		return value, nil
	}
	sources[key] = SourceDefault
	return fallback, nil
}

func FindRepoRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve start path: %w", err)
	}

	for {
		if isRepoRoot(current) {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("repository root not found from current working directory")
		}
		current = parent
	}
}

func isRepoRoot(dir string) bool {
	return fileExists(filepath.Join(dir, "go.mod"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func mysqlDSN(env Env, dbName string) string {
	query := strings.TrimPrefix(env.DatabaseConfig, "?")
	if query == "" {
		return fmt.Sprintf("%s:%s@tcp(%s)/%s", env.DatabaseUsername, env.DatabasePassword, env.DatabaseAddr, dbName)
	}
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?%s", env.DatabaseUsername, env.DatabasePassword, env.DatabaseAddr, dbName, query)
}

func randomSuffix() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano())[:8]
	}
	return hex.EncodeToString(buf)
}

func sanitizeIdentifier(value string, separator rune, fallback string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return fallback
	}

	var builder strings.Builder
	lastWasSeparator := false
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastWasSeparator = false
		default:
			if !lastWasSeparator {
				builder.WriteRune(separator)
				lastWasSeparator = true
			}
		}
	}

	result := strings.Trim(builder.String(), string(separator))
	if result == "" {
		return fallback
	}
	return result
}

func sourceOrUnset(sources map[string]Source, key string) string {
	if source, ok := sources[key]; ok {
		return string(source)
	}
	return "unset"
}

func valueOrUnset(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<unset>"
	}
	return value
}

func redactSecret(value string) string {
	if value == "" {
		return "<empty>"
	}
	return "<redacted>"
}
