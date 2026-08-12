package healthcheck

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReadinessRejectsClosedDatabase(t *testing.T) {
	db := openHealthcheckDatabase(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = NewReadiness(db, nil, false).Check(context.Background())

	require.ErrorContains(t, err, "database")
}

func TestReadinessRejectsUnavailableRequiredRedis(t *testing.T) {
	db := openHealthcheckDatabase(t)
	redisClient := unavailableRedisClient(t)

	err := NewReadiness(db, redisClient, true).Check(context.Background())

	require.ErrorContains(t, err, "redis")
}

func TestReadinessAllowsExplicitlyDisabledRedis(t *testing.T) {
	db := openHealthcheckDatabase(t)

	require.NoError(t, NewReadiness(db, nil, false).Check(context.Background()))
}

func openHealthcheckDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func unavailableRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	client := redis.NewClient(&redis.Options{
		Addr:         address,
		MaxRetries:   -1,
		DialTimeout:  20 * time.Millisecond,
		ReadTimeout:  20 * time.Millisecond,
		WriteTimeout: 20 * time.Millisecond,
	})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}
