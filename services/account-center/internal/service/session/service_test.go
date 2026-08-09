package session_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"paigram/internal/model"
	"paigram/internal/service/session"
)

var dbCounter atomic.Uint64

// newTestDB returns a SQLite in-memory *gorm.DB seeded with just the
// user_sessions table. We use raw DDL because model.UserSession + its
// Production timestamp defaults are not portable to SQLite.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), dbCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS user_sessions (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id             INTEGER NOT NULL,
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

// newTestContext returns a fresh gin.Context bound to a recording
// httptest.ResponseRecorder. Caller controls the headers / IP via the
// returned *http.Request.
func newTestContext(t *testing.T, headers map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "203.0.113.42:12345"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	return c, w
}

func TestIssue_PersistsRowAndSetsCookie(t *testing.T) {
	db := newTestDB(t)
	svc := session.NewService(db, zap.NewNop(),
		session.WithSecureCookie(false), // tests use http://
	)
	c, w := newTestContext(t, map[string]string{
		"User-Agent": "Mozilla/5.0 acceptance",
	})

	require.NoError(t, svc.Issue(c, 42))

	var row model.UserSession
	require.NoError(t, db.Where("user_id = ?", 42).First(&row).Error)
	assert.Equal(t, uint64(42), row.UserID)
	assert.NotEmpty(t, row.AccessTokenHash)
	assert.NotEmpty(t, row.RefreshTokenHash)
	assert.True(t, row.AccessExpiry.After(time.Now()), "access expiry is in the future")
	assert.True(t, row.RefreshExpiry.After(row.AccessExpiry), "refresh outlives access")
	assert.Equal(t, "Mozilla/5.0 acceptance", row.UserAgent)
	assert.Equal(t, "203.0.113.42", row.ClientIP)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1, "exactly one Set-Cookie")
	cookie := cookies[0]
	assert.Equal(t, session.SessionCookieName, cookie.Name)
	assert.True(t, cookie.HttpOnly, "cookie must be HttpOnly")
	assert.False(t, cookie.Secure, "tests opted out of Secure")
	assert.Equal(t, "/", cookie.Path)

	// The cookie value is the raw token; its SHA-256 must equal the hash
	// persisted in the DB row.
	sum := sha256.Sum256([]byte(cookie.Value))
	assert.Equal(t, hex.EncodeToString(sum[:]), row.AccessTokenHash)
}

func TestIssue_SecureCookieDefault(t *testing.T) {
	db := newTestDB(t)
	svc := session.NewService(db, zap.NewNop())
	c, w := newTestContext(t, nil)

	require.NoError(t, svc.Issue(c, 1))

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.True(t, cookies[0].Secure, "Secure attribute must default to true")
}

func TestIssue_RejectsZeroUser(t *testing.T) {
	db := newTestDB(t)
	svc := session.NewService(db, zap.NewNop())
	c, _ := newTestContext(t, nil)

	err := svc.Issue(c, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zero user id")
}

func TestIssue_RejectsNilContext(t *testing.T) {
	db := newTestDB(t)
	svc := session.NewService(db, zap.NewNop())

	err := svc.Issue(nil, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil gin context")
}

func TestIssue_HonorsTTLOptions(t *testing.T) {
	db := newTestDB(t)
	svc := session.NewService(db, zap.NewNop(),
		session.WithAccessTTL(3*time.Minute),
		session.WithRefreshTTL(2*time.Hour),
	)
	c, w := newTestContext(t, nil)
	require.NoError(t, svc.Issue(c, 7))

	var row model.UserSession
	require.NoError(t, db.Where("user_id = ?", 7).First(&row).Error)
	assert.WithinDuration(t, time.Now().Add(3*time.Minute), row.AccessExpiry, 5*time.Second)
	assert.WithinDuration(t, time.Now().Add(2*time.Hour), row.RefreshExpiry, 5*time.Second)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, 180, cookies[0].MaxAge, "cookie Max-Age matches access TTL in seconds")
}

func TestIssue_UserAgentTruncation(t *testing.T) {
	db := newTestDB(t)
	svc := session.NewService(db, zap.NewNop())
	longUA := strings.Repeat("U", 600) // user_sessions.user_agent is varchar(512)
	c, _ := newTestContext(t, map[string]string{"User-Agent": longUA})
	require.NoError(t, svc.Issue(c, 9))

	var row model.UserSession
	require.NoError(t, db.Where("user_id = ?", 9).First(&row).Error)
	assert.Len(t, row.UserAgent, 512, "User-Agent truncated to column width")
}

// TestIssue_CookieAttributes_SameSiteLax asserts the explicit SameSite
// attribute. SameSite=Lax is required so the 302 redirect from
// oauth.telegram.org back to /api/v1/auth/telegram/callback carries the
// session cookie on the cross-site landing navigation. Strict would
// silently drop it; None requires Secure and over-shares. Without an
// explicit attribute the browser default varies across vendors.
//
// We parse the raw Set-Cookie header string rather than the
// http.Cookie struct because Go's stdlib http.Response.Cookies()
// preserves SameSite, but a string-level assertion is the clearest
// regression guard against an accidental switch back to gin's
// c.SetCookie helper (which does NOT expose SameSite).
func TestIssue_CookieAttributes_SameSiteLax(t *testing.T) {
	db := newTestDB(t)
	svc := session.NewService(db, zap.NewNop(),
		session.WithSecureCookie(false),
	)
	c, w := newTestContext(t, nil)

	require.NoError(t, svc.Issue(c, 7))

	cookieHdr := w.Header().Get("Set-Cookie")
	require.NotEmpty(t, cookieHdr, "Set-Cookie header must be present")
	assert.Contains(t, cookieHdr, "SameSite=Lax",
		"Set-Cookie must carry SameSite=Lax for cross-site OIDC redirect compatibility")
	assert.Contains(t, cookieHdr, "HttpOnly",
		"Set-Cookie must carry HttpOnly so JS cannot read the session token")
	assert.Contains(t, cookieHdr, "Path=/",
		"Set-Cookie must scope to Path=/ so all API routes see the cookie")
}

// TestIssue_NoRefreshTokenSurface asserts the orphan-refresh defect from
// A4.1-C is fixed: no Refresh()/Rotate() API exists, no refresh material
// is returned to the caller, and the persisted RefreshTokenHash is an
// opaque nonce (non-empty to satisfy the column's NOT NULL + UNIQUE
// constraint) that cannot be matched against anything client-supplied.
// See package doc for the rationale.
func TestIssue_NoRefreshTokenSurface(t *testing.T) {
	db := newTestDB(t)
	svc := session.NewService(db, zap.NewNop(),
		session.WithSecureCookie(false),
	)
	c, w := newTestContext(t, nil)

	require.NoError(t, svc.Issue(c, 99))

	// Exactly one Set-Cookie, named SessionCookieName — no separate
	// refresh-token cookie.
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1, "exactly one Set-Cookie; no refresh cookie")
	assert.Equal(t, session.SessionCookieName, cookies[0].Name)

	// Body is empty (this is a redirect-only flow); no JSON refresh token.
	assert.Empty(t, w.Body.String(), "Issue must not write a body / refresh token")

	// The persisted refresh_token_hash is the opaque placeholder — non-empty
	// (per the NOT NULL + UNIQUE column constraint) but never returned to
	// the client and never matched against any input.
	var row model.UserSession
	require.NoError(t, db.Where("user_id = ?", 99).First(&row).Error)
	assert.NotEmpty(t, row.RefreshTokenHash,
		"placeholder must be non-empty to satisfy NOT NULL + UNIQUE")
}
