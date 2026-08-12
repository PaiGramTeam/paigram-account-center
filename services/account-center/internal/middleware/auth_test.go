package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"paigram/internal/config"
	"paigram/internal/model"
	"paigram/internal/response"
	"paigram/internal/service/session"
	"paigram/internal/sessioncache"
)

var authTestDBCounter atomic.Uint64

// setupAuthMiddlewareDB builds an in-memory SQLite DB with the schemas
// AuthMiddleware needs (users + user_sessions) and a per-test DSN so
// parallel reruns (count=N) do not share state.
//
// Raw DDL is used rather than AutoMigrate because model.User declares
// database-specific column defaults that
// SQLite cannot parse. The model.UserSession schema is mirrored
// directly from service/session.newTestDB for consistency.
func setupAuthMiddlewareDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:auth_mw_%s_%d?mode=memory&cache=shared",
		t.Name(), authTestDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			user_ref            TEXT    NOT NULL,
			owner_epoch         INTEGER NOT NULL DEFAULT 1,
			primary_login_type  TEXT    NOT NULL DEFAULT 'email',
			status              TEXT    NOT NULL DEFAULT 'active',
			primary_role_id     INTEGER,
			last_login_at       DATETIME,
			created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at          DATETIME
		)`).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_users_ref ON users(user_ref)`,
	).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS user_sessions (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id             INTEGER NOT NULL,
			family_id           TEXT    NOT NULL DEFAULT '',
			access_token_hash   TEXT    NOT NULL,
			refresh_token_hash  TEXT    NOT NULL,
			access_expiry       DATETIME NOT NULL,
			refresh_expiry      DATETIME NOT NULL,
			user_agent          TEXT,
			client_ip           TEXT,
			created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			revoked_at          DATETIME,
			revoked_reason      TEXT
		)`).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_user_sessions_access ON user_sessions(access_token_hash)`,
	).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_user_sessions_refresh ON user_sessions(refresh_token_hash)`,
	).Error)
	return db
}

// seedActiveUserAndSession creates an active user + a single session row
// keyed by accessToken (SHA-256 hashed). Returns the user ID for assertions.
func seedActiveUserAndSession(t *testing.T, db *gorm.DB, accessToken string) uint64 {
	t.Helper()
	user := model.User{
		PrimaryLoginType: model.LoginTypeEmail,
		Status:           model.UserStatusActive,
	}
	require.NoError(t, db.Create(&user).Error)

	now := time.Now().UTC()
	row := model.UserSession{
		UserID:           user.ID,
		AccessTokenHash:  hashToken(accessToken),
		RefreshTokenHash: hashToken("refresh-" + accessToken),
		AccessExpiry:     now.Add(time.Hour),
		RefreshExpiry:    now.Add(24 * time.Hour),
		UserAgent:        "test-agent",
		ClientIP:         "203.0.113.99",
		// Keep UpdatedAt recent so AuthMiddleware does not enter the
		// session-refresh branch and emit extra DB writes during this
		// test. Refresh logic is covered by other tests.
		UpdatedAt: now,
	}
	require.NoError(t, db.Create(&row).Error)
	return user.ID
}

// buildAuthMiddlewareRouter wires AuthMiddleware in front of a no-op
// handler that echoes the userID set on the gin context. The handler
// returning 200 with the userID is the contract verified by every
// cookie-fallback assertion below.
func buildAuthMiddlewareRouter() *gin.Engine {
	authCfg := config.AuthConfig{
		AccessTokenTTLSeconds:   900,
		RefreshTokenTTLSeconds:  7 * 24 * 3600,
		SessionUpdateAgeSeconds: 24 * 3600,
	}
	router := gin.New()
	router.GET("/whoami",
		AuthMiddleware(sessioncache.NewNoopStore(), authCfg),
		func(c *gin.Context) {
			uid, _ := GetUserID(c)
			response.Success(c, gin.H{"user_id": uid})
		},
	)
	return router
}

func decodeAuthErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	errPayload, ok := payload["error"].(map[string]any)
	require.True(t, ok, "response missing error envelope: %s", recorder.Body.String())
	code, _ := errPayload["code"].(string)
	return code
}

func decodeAuthSuccessUserID(t *testing.T, recorder *httptest.ResponseRecorder) float64 {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok, "response missing data envelope: %s", recorder.Body.String())
	uid, ok := data["user_id"].(float64)
	require.True(t, ok, "user_id missing or wrong type: %v", data)
	return uid
}

// TestAuthMiddleware_CookieFallback exercises the spec §6.1 change that
// AuthMiddleware accepts the HttpOnly session cookie issued by
// service/session.Service.Issue as a fallback when no Authorization
// header is present, while preserving the existing Bearer-header
// contract.
func TestAuthMiddleware_CookieFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("cookie only succeeds and reaches handler", func(t *testing.T) {
		db := setupAuthMiddlewareDB(t)
		restore := setMiddlewareTestServiceGroup(t, db)
		defer restore()

		const token = "cookie-token-ok-1"
		userID := seedActiveUserAndSession(t, db, token)

		router := buildAuthMiddlewareRouter()
		req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
		req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: token})
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		require.Equalf(t, http.StatusOK, recorder.Code, "body: %s", recorder.Body.String())
		assert.Equal(t, float64(userID), decodeAuthSuccessUserID(t, recorder))
	})

	t.Run("no header and no cookie returns MISSING_TOKEN", func(t *testing.T) {
		db := setupAuthMiddlewareDB(t)
		restore := setMiddlewareTestServiceGroup(t, db)
		defer restore()

		router := buildAuthMiddlewareRouter()
		req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		assert.Equal(t, "MISSING_TOKEN", decodeAuthErrorCode(t, recorder))
	})

	t.Run("header takes precedence when both present", func(t *testing.T) {
		db := setupAuthMiddlewareDB(t)
		restore := setMiddlewareTestServiceGroup(t, db)
		defer restore()

		// Seed a session for the BEARER token only. The cookie carries
		// a value that has no matching session row; if the middleware
		// (incorrectly) used the cookie, the request would 401.
		const bearerToken = "bearer-token-wins-1"
		const cookieToken = "stale-cookie-should-be-ignored"
		userID := seedActiveUserAndSession(t, db, bearerToken)

		router := buildAuthMiddlewareRouter()
		req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
		req.Header.Set("Authorization", "Bearer "+bearerToken)
		req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: cookieToken})
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		require.Equalf(t, http.StatusOK, recorder.Code, "body: %s", recorder.Body.String())
		assert.Equal(t, float64(userID), decodeAuthSuccessUserID(t, recorder))
	})

	t.Run("malformed authorization header does not silently fall through to cookie", func(t *testing.T) {
		db := setupAuthMiddlewareDB(t)
		restore := setMiddlewareTestServiceGroup(t, db)
		defer restore()

		// Even if a valid cookie is also present, a malformed
		// Authorization header MUST 401 with INVALID_TOKEN_FORMAT.
		// Otherwise a buggy / hostile client could probe Bearer parsing
		// and then silently downgrade to the cookie auth path.
		const cookieToken = "valid-cookie-but-header-wins"
		seedActiveUserAndSession(t, db, cookieToken)

		router := buildAuthMiddlewareRouter()
		req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: cookieToken})
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		assert.Equal(t, "INVALID_TOKEN_FORMAT", decodeAuthErrorCode(t, recorder))
	})
}

// TestAuthMiddleware_BearerStillWorks is a regression guard on the
// pre-existing Bearer-only contract. Cookie-fallback work must not
// change the path that classic API clients use.
func TestAuthMiddleware_BearerStillWorks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupAuthMiddlewareDB(t)
	restore := setMiddlewareTestServiceGroup(t, db)
	defer restore()

	const token = "bearer-only-regression"
	userID := seedActiveUserAndSession(t, db, token)

	router := buildAuthMiddlewareRouter()
	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equalf(t, http.StatusOK, recorder.Code, "body: %s", recorder.Body.String())
	assert.Equal(t, float64(userID), decodeAuthSuccessUserID(t, recorder))
}

// TestAuthMiddleware_EmptyBearerToken guards the EMPTY_TOKEN code path:
// an Authorization header with `Bearer ` (no token) must still 401.
func TestAuthMiddleware_EmptyBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupAuthMiddlewareDB(t)
	restore := setMiddlewareTestServiceGroup(t, db)
	defer restore()

	router := buildAuthMiddlewareRouter()
	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.Header.Set("Authorization", "Bearer ")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Equal(t, "EMPTY_TOKEN", decodeAuthErrorCode(t, recorder))
}
