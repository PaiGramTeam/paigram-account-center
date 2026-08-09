package integrationenv

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

const (
	envFileName        = ".env.integration.local"
	defaultRedisPrefix = "itest"
	defaultRedisDB     = 0
	connectTimeout     = 10 * time.Second
)

type Source string

const (
	SourceDefault Source = "default"
	SourceFile    Source = "file"
	SourceShell   Source = "shell"
)

type Sources struct {
	DatabaseDSN           Source
	RedisAddr             Source
	RedisCredentialOrigin Source
	RedisDB               Source
	RedisPrefix           Source
}

type Env struct {
	RepoRoot      string
	EnvFilePath   string
	EnvFileLoaded bool
	GoWork        string

	DatabaseDSN   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RedisPrefix   string

	HasDatabaseDSN   bool
	HasRedisPassword bool

	Sources Sources
}

type LoadOptions struct {
	WorkingDir string
	LookupEnv  func(string) (string, bool)
}

func Load(opts LoadOptions) (Env, error) {
	workingDir := strings.TrimSpace(opts.WorkingDir)
	if workingDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Env{}, fmt.Errorf("resolve working directory: %w", err)
		}
		workingDir = cwd
	}

	repoRoot, err := findRepoRoot(workingDir)
	if err != nil {
		return Env{}, err
	}
	lookupEnv := opts.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	envFilePath := filepath.Join(repoRoot, envFileName)
	fileValues, loaded, err := loadEnvFile(envFilePath)
	if err != nil {
		return Env{}, err
	}

	env := Env{
		RepoRoot:      repoRoot,
		EnvFilePath:   envFilePath,
		EnvFileLoaded: loaded,
		GoWork:        readOptional(lookupEnv, "GOWORK"),
	}
	env.DatabaseDSN, env.Sources.DatabaseDSN = selectString(lookupEnv, fileValues, "PAI_TEST_DATABASE_DSN", "")
	env.RedisAddr, env.Sources.RedisAddr = selectString(lookupEnv, fileValues, "PAI_TEST_REDIS_ADDR", "")
	env.RedisPassword, env.Sources.RedisCredentialOrigin = selectString(lookupEnv, fileValues, "PAI_TEST_REDIS_PASSWORD", "")
	env.RedisPrefix, env.Sources.RedisPrefix = selectString(lookupEnv, fileValues, "PAI_TEST_REDIS_PREFIX", defaultRedisPrefix)
	env.HasDatabaseDSN = strings.TrimSpace(env.DatabaseDSN) != ""
	env.HasRedisPassword = strings.TrimSpace(env.RedisPassword) != ""

	redisDB, source, err := selectInt(lookupEnv, fileValues, "PAI_TEST_REDIS_DB", defaultRedisDB)
	if err != nil {
		return Env{}, err
	}
	env.RedisDB = redisDB
	env.Sources.RedisDB = source
	return env, nil
}

func (e Env) MissingRequired() []string {
	missing := make([]string, 0, 2)
	if strings.TrimSpace(e.DatabaseDSN) == "" {
		missing = append(missing, "PAI_TEST_DATABASE_DSN")
	}
	if strings.TrimSpace(e.RedisAddr) == "" {
		missing = append(missing, "PAI_TEST_REDIS_ADDR")
	}
	return missing
}

func (e Env) SummaryLines(sampleName string, requireRedis bool) []string {
	lines := []string{
		"repo_root=" + e.RepoRoot,
		fmt.Sprintf("env_file=%s (%s)", e.EnvFilePath, envFileState(e.EnvFileLoaded)),
		fmt.Sprintf("postgres.dsn=%s (%s)", credentialTag(e.HasDatabaseDSN), e.Sources.DatabaseDSN),
		fmt.Sprintf("redis.required=%t", requireRedis),
		fmt.Sprintf("redis.addr=%s (%s)", displayValue(e.RedisAddr), e.Sources.RedisAddr),
		fmt.Sprintf("redis.password=%s (%s)", credentialTag(e.HasRedisPassword), e.Sources.RedisCredentialOrigin),
		fmt.Sprintf("redis.db=%d (%s)", e.RedisDB, e.Sources.RedisDB),
		fmt.Sprintf("redis.prefix=%s (%s)", displayValue(e.RedisPrefix), e.Sources.RedisPrefix),
		"gowork=" + displayGoWork(e.GoWork),
	}
	if strings.TrimSpace(sampleName) != "" {
		lines = append(lines,
			"sample.postgres.database="+e.UniqueDatabaseName(sampleName),
			"sample.redis.prefix="+e.UniqueRedisPrefix(sampleName),
		)
	}
	return lines
}

func (e Env) UniqueDatabaseName(testName string) string {
	baseName := "paigram"
	if cfg, err := pgx.ParseConfig(e.DatabaseDSN); err == nil && strings.TrimSpace(cfg.Database) != "" {
		baseName = sanitizeName(cfg.Database)
	}
	if len(baseName) > 24 {
		baseName = baseName[:24]
	}
	return fmt.Sprintf("t_%s_%s_%s", baseName, shortHash(testName), shortHash(fmt.Sprintf("%d", time.Now().UnixNano())))
}

func (e Env) UniqueRedisPrefix(testName string) string {
	baseName := sanitizeName(e.RedisPrefix)
	if len(baseName) > 16 {
		baseName = baseName[:16]
	}
	return fmt.Sprintf("%s:%s:%s", baseName, shortHash(testName), shortHash(fmt.Sprintf("%d", time.Now().UnixNano())))
}

func (e Env) CheckPostgreSQL(ctx context.Context) error {
	if ctx == nil {
		localCtx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		ctx = localCtx
	}
	db, err := sql.Open("pgx", e.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("open PostgreSQL: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return nil
}

func (e Env) CheckRedis(ctx context.Context) error {
	if ctx == nil {
		localCtx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		ctx = localCtx
	}
	client := redis.NewClient(&redis.Options{Addr: e.RedisAddr, Password: e.RedisPassword, DB: e.RedisDB})
	defer client.Close()
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}
	return nil
}

func sanitizeName(value string) string {
	value = strings.ToLower(value)
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", "-", "_", ":", "_", ".", "_")
	value = replacer.Replace(value)
	value = strings.Trim(value, "_")
	if value == "" {
		return defaultRedisPrefix
	}
	return value
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func envFileState(loaded bool) string {
	if loaded {
		return "loaded"
	}
	return "missing"
}

func displayGoWork(value string) string {
	if strings.TrimSpace(value) == "" {
		return "off"
	}
	return strings.TrimSpace(value)
}

func displayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<empty>"
	}
	return strings.TrimSpace(value)
}

func credentialTag(present bool) string {
	if !present {
		return "<empty>"
	}
	return "<redacted>"
}

func selectString(lookupEnv func(string) (string, bool), fileValues map[string]string, key, fallback string) (string, Source) {
	if value, ok := lookupEnv(key); ok {
		return value, SourceShell
	}
	if value, ok := fileValues[key]; ok {
		return value, SourceFile
	}
	return fallback, SourceDefault
}

func selectInt(lookupEnv func(string) (string, bool), fileValues map[string]string, key string, fallback int) (int, Source, error) {
	if value, ok := lookupEnv(key); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, "", fmt.Errorf("parse %s from shell: %w", key, err)
		}
		return parsed, SourceShell, nil
	}
	if value, ok := fileValues[key]; ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, "", fmt.Errorf("parse %s from file: %w", key, err)
		}
		return parsed, SourceFile, nil
	}
	return fallback, SourceDefault, nil
}

func readOptional(lookupEnv func(string) (string, bool), key string) string {
	if value, ok := lookupEnv(key); ok {
		return value
	}
	return ""
}

func loadEnvFile(path string) (map[string]string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, false, nil
		}
		return nil, false, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, false, fmt.Errorf("parse %s line %d: expected KEY=VALUE", path, lineNumber)
		}
		values[strings.TrimSpace(key)] = unquote(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, false, fmt.Errorf("scan %s: %w", path, err)
	}
	return values, true, nil
}

func unquote(value string) string {
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
		return value[1 : len(value)-1]
	}
	return value
}

func findRepoRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil && !info.IsDir() {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("could not find repo root from %s", start)
		}
		current = parent
	}
}
