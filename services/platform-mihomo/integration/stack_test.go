//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"platform-mihomo-service/integration/testenv"
)

type integrationStack struct {
	DatabaseName string
	SQLDB        *sql.DB
	DB           *gorm.DB
	Redis        *redis.Client
	RedisPrefix  string
}

func TestNewIntegrationStackCreatesUniqueDatabasePerTestStack(t *testing.T) {
	first := newIntegrationStack(t)
	second := newIntegrationStack(t)

	require.NotEqual(t, first.DatabaseName, second.DatabaseName)
}

func newIntegrationStack(t *testing.T) *integrationStack {
	t.Helper()
	return newIntegrationStackWithDatabase(t, testenv.NewPerTestDB(t))
}

func newEmptyIntegrationStack(t *testing.T) *integrationStack {
	t.Helper()
	return newIntegrationStackWithDatabase(t, testenv.NewEmptyPerTestDB(t))
}

func newIntegrationStackWithDatabase(t *testing.T, database *testenv.PerTestDB) *integrationStack {
	t.Helper()
	stack := &integrationStack{
		DatabaseName: database.Name,
		SQLDB:        database.SQL,
		DB:           database.GORM,
		RedisPrefix:  uniqueRedisPrefix(t.Name()),
	}
	stack.Redis = openRedis(t, testenv.MustShared().RedisAddr)
	cleanupRedisPrefix(t, stack.Redis, stack.RedisPrefix)
	t.Cleanup(func() {
		cleanupRedisPrefix(t, stack.Redis, stack.RedisPrefix)
		_ = stack.Redis.Close()
	})
	return stack
}

func openRedis(t *testing.T, address string) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: address})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx).Err())
	return client
}

func cleanupRedisPrefix(t *testing.T, client *redis.Client, prefix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var cursor uint64
	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, prefix+"*", 100).Result()
		require.NoError(t, err)
		if len(keys) > 0 {
			require.NoError(t, client.Del(ctx, keys...).Err())
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

func uniqueRedisPrefix(testName string) string {
	hash := sha256.Sum256([]byte(testName + time.Now().UTC().String()))
	return "itest:" + hex.EncodeToString(hash[:8]) + ":"
}
