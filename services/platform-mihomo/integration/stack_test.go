//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/testutil/integrationenv"
)

type integrationStack struct {
	SQLDB       *sql.DB
	Redis       *redis.Client
	DatabaseCfg databaseConfig
	RedisPrefix string
	cleanup     func()
}

type databaseConfig struct {
	Source string
	Dbname string
}

func TestNewIntegrationStackCreatesUniqueDatabasePerTestStack(t *testing.T) {
	first := newIntegrationStack(t)
	t.Cleanup(first.cleanup)

	second := newIntegrationStack(t)
	t.Cleanup(second.cleanup)

	require.NotEqual(t, first.DatabaseCfg.Dbname, second.DatabaseCfg.Dbname)
}

func TestValidateDatabaseConfigForMigrationsRequiresMultiStatements(t *testing.T) {
	err := validateDatabaseConfigForMigrations("charset=utf8mb4&parseTime=True")
	require.Error(t, err)
	require.Contains(t, err.Error(), "multiStatements=true")
}

func TestValidateDatabaseConfigForMigrationsAcceptsEnabledMultiStatements(t *testing.T) {
	err := validateDatabaseConfigForMigrations("charset=utf8mb4&parseTime=True&multiStatements=true")
	require.NoError(t, err)
}

func newIntegrationStack(t *testing.T) *integrationStack {
	t.Helper()

	env := loadIntegrationEnv(t)
	dbName := integrationenv.UniqueDatabaseName(env.DatabaseBaseName, t.Name())
	redisPrefix := normalizeRedisPrefix(integrationenv.UniqueRedisPrefix(env.RedisPrefix, t.Name()))
	t.Logf("integration stack database=%s redis_prefix=%s", dbName, redisPrefix)

	serverDB, err := openDatabase(databaseSource(env, ""))
	if err != nil {
		t.Skipf("integration database unavailable: %v", err)
	}

	ensureDatabaseExists(t, serverDB, dbName)

	appSource := databaseSource(env, dbName)
	appDB, err := openDatabase(appSource)
	if err != nil {
		_ = dropDatabase(serverDB, dbName)
		_ = serverDB.Close()
		t.Skipf("integration database unavailable: %v", err)
	}

	redisClient, err := openRedis(env)
	if err != nil {
		_ = appDB.Close()
		_ = dropDatabase(serverDB, dbName)
		_ = serverDB.Close()
		t.Skipf("integration redis unavailable: %v", err)
	}

	return &integrationStack{
		SQLDB: appDB,
		Redis: redisClient,
		DatabaseCfg: databaseConfig{
			Source: appSource,
			Dbname: dbName,
		},
		RedisPrefix: redisPrefix,
		cleanup: func() {
			if redisClient != nil {
				cleanupRedisPrefix(t, redisClient, redisPrefix)
				_ = redisClient.Close()
			}
			_ = appDB.Close()
			if err := dropDatabase(serverDB, dbName); err != nil {
				t.Fatalf("drop database %q: %v", dbName, err)
			}
			_ = serverDB.Close()
		},
	}
}

func loadIntegrationEnv(t *testing.T) integrationenv.Env {
	t.Helper()

	env, err := integrationenv.Load(repoRoot(t))
	if err != nil {
		t.Fatalf("load integration env: %v", err)
	}
	if strings.TrimSpace(env.DatabaseAddr) == "" {
		t.Skip("PAI_TEST_DATABASE_ADDR is required for integration database tests")
	}
	if strings.TrimSpace(env.DatabaseUsername) == "" {
		t.Skip("PAI_TEST_DATABASE_USERNAME is required for integration database tests")
	}
	if strings.TrimSpace(env.DatabaseBaseName) == "" {
		t.Skip("PAI_TEST_DATABASE_DBNAME is required for integration database tests")
	}
	if err := validateDatabaseConfigForMigrations(env.DatabaseConfig); err != nil {
		t.Fatal(err)
	}

	return env
}

func openDatabase(source string) (*sql.DB, error) {
	db, err := sql.Open("mysql", source)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

func ensureDatabaseExists(t *testing.T, db *sql.DB, dbName string) {
	t.Helper()

	if _, err := db.Exec("CREATE DATABASE `" + dbName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create database %q: %v", dbName, err)
	}
}

func dropDatabase(db *sql.DB, dbName string) error {
	_, err := db.Exec("DROP DATABASE IF EXISTS `" + dbName + "`")
	if err != nil {
		return fmt.Errorf("drop database %q: %w", dbName, err)
	}
	return nil
}

func openRedis(env integrationenv.Env) (*redis.Client, error) {
	if strings.TrimSpace(env.RedisAddr) == "" {
		return nil, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     env.RedisAddr,
		Password: env.RedisPassword,
		DB:       env.RedisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}

func cleanupRedisPrefix(t *testing.T, client *redis.Client, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pattern := prefix + "*"
	var cursor uint64
	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			t.Fatalf("scan redis keys for prefix %q: %v", prefix, err)
		}
		if len(keys) > 0 {
			if err := client.Del(ctx, keys...).Err(); err != nil {
				t.Fatalf("delete redis keys for prefix %q: %v", prefix, err)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

func databaseSource(env integrationenv.Env, dbName string) string {
	query := strings.TrimPrefix(env.DatabaseConfig, "?")
	if query == "" {
		return fmt.Sprintf("%s:%s@tcp(%s)/%s", env.DatabaseUsername, env.DatabasePassword, env.DatabaseAddr, dbName)
	}
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?%s", env.DatabaseUsername, env.DatabasePassword, env.DatabaseAddr, dbName, query)
}

func validateDatabaseConfigForMigrations(config string) error {
	values, err := url.ParseQuery(strings.TrimPrefix(config, "?"))
	if err != nil {
		return fmt.Errorf("parse PAI_TEST_DATABASE_CONFIG: %w", err)
	}
	if !strings.EqualFold(values.Get("multiStatements"), "true") {
		return fmt.Errorf("PAI_TEST_DATABASE_CONFIG must include multiStatements=true so integration migrations can execute multi-statement SQL")
	}
	return nil
}

func normalizeRedisPrefix(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, ":") {
		return trimmed
	}
	return trimmed + ":"
}
