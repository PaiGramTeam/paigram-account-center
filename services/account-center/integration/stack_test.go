//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	initmigrate "paigram/initialize/migrate"
	"paigram/integration/testenv"
	"paigram/internal/casbin"
	"paigram/internal/config"
	"paigram/internal/crypto"
	"paigram/internal/email"
	"paigram/internal/middleware"
	"paigram/internal/model"
	"paigram/internal/platformtransport"
	"paigram/internal/router"
	"paigram/internal/sessioncache"
	"paigram/internal/testutil"
)

type integrationStack struct {
	DatabaseCfg config.DatabaseConfig
	Schema      string
	SQLDB       *sql.DB
	DB          *gorm.DB
	Redis       *redis.Client
	RedisPrefix string
	Email       *email.Service
	Router      http.Handler
	ControlTLS  tlstest.Bundle
	ControlDial platformtransport.DialFunc
}

func newIntegrationStack(t *testing.T) *integrationStack {
	return newIntegrationStackWithConfig(t, nil)
}

func newIntegrationStackWithConfig(t *testing.T, mutate func(*config.Config)) *integrationStack {
	t.Helper()
	require.NoError(t, crypto.SetEncryptionKey([]byte("0123456789abcdef0123456789abcdef")))

	shared := testenv.MustShared()
	database := testenv.NewPerTestDB(t)
	stack := &integrationStack{
		DatabaseCfg: config.DatabaseConfig{
			DSN:           database.DSN,
			MigrationsDir: migrationsDir(),
			AutoMigrate:   false,
			AutoSeed:      false,
		},
		Schema:      "public",
		SQLDB:       database.SQL,
		DB:          database.GORM,
		RedisPrefix: uniqueRedisPrefix(t.Name()),
	}

	stack.Redis = openRedis(t, shared.RedisAddr)
	cleanupRedisPrefix(t, stack.Redis, stack.RedisPrefix)
	t.Cleanup(func() {
		cleanupRedisPrefix(t, stack.Redis, stack.RedisPrefix)
		_ = stack.Redis.Close()
	})

	emailService, err := email.NewService(config.EmailConfig{Enabled: false})
	require.NoError(t, err)
	stack.Email = emailService
	t.Cleanup(func() { _ = stack.Email.Close() })

	sessionStore := sessioncache.NewRedisStore(stack.Redis, stack.RedisPrefix)
	rateLimitStore, err := middleware.NewRedisStore(stack.Redis, stack.RedisPrefix+":ratelimit")
	require.NoError(t, err)

	stack.ControlTLS = tlstest.New(t, "control.internal")
	testCfg := newTestConfigWithControlTLS(t, stack.RedisPrefix, stack.ControlTLS)
	if mutate != nil {
		mutate(testCfg)
	}
	stack.ControlDial, err = platformtransport.NewControlDialer(platformtransport.ControlConfig{
		RootCAFile: testCfg.PlatformControl.RootCAFile, CertificateFile: testCfg.PlatformControl.CertificateFile,
		PrivateKeyFile: testCfg.PlatformControl.PrivateKeyFile, ServerName: testCfg.PlatformControl.ServerName,
		Timeout: testCfg.PlatformControl.DialTimeout,
	})
	require.NoError(t, err)
	stack.Router, err = router.New(testCfg, sessionStore, stack.DB, rateLimitStore, stack.Email)
	require.NoError(t, err)

	casbin.Reset()
	_, err = casbin.InitEnforcer(stack.DB)
	require.NoError(t, err, "initialize Casbin enforcer")
	return stack
}

func newTestConfig(t testing.TB, redisPrefix string) *config.Config {
	return newTestConfigWithControlTLS(t, redisPrefix, tlstest.New(t, "control.internal"))
}

func newTestConfigWithControlTLS(t testing.TB, redisPrefix string, controlTLS tlstest.Bundle) *config.Config {
	authConfig, _ := testutil.NewAuthConfig(t)
	authConfig.EmailVerificationTTLSeconds = 86400
	authConfig.SessionUpdateAgeSeconds = 86400
	authConfig.SessionFreshAgeSeconds = 300
	authConfig.RequireEmailVerificationLogin = true
	return &config.Config{
		App: config.AppConfig{
			Name:           "Paigram Integration Test",
			Mode:           "test",
			TrustedProxies: []string{"127.0.0.1"},
			IPv6Subnet:     64,
		},
		OpenAPI: config.OpenAPIConfig{Enabled: true, Path: "/openapi"},
		Auth:    authConfig,
		PlatformControl: config.PlatformControlConfig{
			RootCAFile: controlTLS.CAFile, CertificateFile: controlTLS.ClientCertFile,
			PrivateKeyFile: controlTLS.ClientKeyFile, ServerName: controlTLS.ServerName,
			DialTimeout: 5 * time.Second,
		},
		Frontend: config.FrontendConfig{
			BaseURL: integrationBrowserOrigin,
		},
		Redis: config.RedisConfig{
			Enabled: true,
			Addr:    testenv.MustShared().RedisAddr,
			DB:      0,
			Prefix:  redisPrefix,
		},
		RateLimit: config.RateLimitConfig{
			Enabled: true,
			Auth: config.RateLimitAuthConfig{
				Login:        "10-M",
				Register:     "10-M",
				VerifyEmail:  "10-M",
				RefreshToken: "10-M",
				OAuth:        "10-M",
			},
			API: config.RateLimitAPIConfig{
				Authenticated:   "100-H",
				Unauthenticated: "100-H",
			},
		},
		Email: config.EmailConfig{Enabled: false},
		Security: config.SecurityConfig{
			SuspiciousLoginDetection:  false,
			SuspiciousLoginEmailAlert: false,
			BcryptCost:                10,
		},
	}
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

func runMigrations(t *testing.T, sqlDB *sql.DB, cfg config.DatabaseConfig) {
	t.Helper()
	require.NoError(t, initmigrate.Run(sqlDB, cfg))
}

func migrationsDir() string {
	return filepath.Join("..", "initialize", "migrate", "sql")
}

func requireTableExists(t *testing.T, db *sql.DB, schema, table string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_name = $2
	`, schema, table).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "expected table %s to exist", table)
}

func uniqueRedisPrefix(testName string) string {
	hash := sha256.Sum256([]byte(testName + time.Now().UTC().String()))
	return "itest:" + hex.EncodeToString(hash[:8])
}

func hashTokenForTest(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func requireSessionForRefreshToken(t *testing.T, db *gorm.DB, refreshToken string) model.UserSession {
	t.Helper()
	var session model.UserSession
	require.NoError(t, db.Where("refresh_token_hash = ?", hashTokenForTest(refreshToken)).First(&session).Error)
	return session
}
