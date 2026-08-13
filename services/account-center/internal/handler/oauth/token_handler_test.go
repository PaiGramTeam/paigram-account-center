package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"paigram/internal/model"
	"paigram/internal/service/credentials"
)

var testSigningKey = []byte("0123456789abcdef0123456789abcdef")

type tokenHandlerFixture struct {
	router      *gin.Engine
	credentials *credentials.Service
	tokens      *credentials.TokenService
	clientID    string
	secret      string
}

// newTokenHandlerFixture spins up a gin engine routing POST /oauth/token at
// a TokenHandler wired against an in-memory sqlite credentials store. A
// single seed credential is created so tests can drive the happy and
// failure paths without each having to do their own setup boilerplate.
//
// The credential tables are created directly via DDL rather than
// AutoMigrate: GORM would walk ServiceCredential's `Owner User`
// association and try to migrate the User model, whose column defaults
// use database-specific types and defaults that SQLite cannot parse.
func newTokenHandlerFixture(t *testing.T) *tokenHandlerFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`DROP TABLE IF EXISTS service_credentials`).Error)
	require.NoError(t, db.Exec(`DROP TABLE IF EXISTS bots`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE bots (
		id TEXT PRIMARY KEY,
		entry_issuer TEXT NOT NULL,
		display_name TEXT NOT NULL,
		description TEXT,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		owner_user_id INTEGER NOT NULL,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX uk_test_bots_entry_issuer ON bots(entry_issuer)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE service_credentials (
		client_id TEXT PRIMARY KEY,
		consumer_epoch INTEGER NOT NULL DEFAULT 1,
		bot_id TEXT NOT NULL,
		display_name TEXT NOT NULL,
		secret_hash TEXT NOT NULL,
		audiences TEXT NOT NULL,
		scopes TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		owner_user_id INTEGER NOT NULL,
		description TEXT,
		last_used_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME
	)`).Error)

	svc := credentials.NewService(db)
	tokenSvc, err := credentials.NewTokenService(svc, credentials.TokenServiceConfig{
		Issuer:                "account-center",
		AccessTokenTTLSeconds: 3600,
		SigningKey:            testSigningKey,
	})
	require.NoError(t, err)

	result, err := svc.Create(credentials.CreateInput{
		ClientID:    "telegram-service",
		DisplayName: "Telegram",
		OwnerUserID: 1,
		Audiences:   []string{"mihomo.sync", "account-center"},
		Scopes:      []string{"binding.read", "binding.write"},
	})
	require.NoError(t, err)

	handler := NewTokenHandler(tokenSvc)
	router := gin.New()
	router.POST("/oauth/token", handler.Token)

	return &tokenHandlerFixture{
		router:      router,
		credentials: svc,
		tokens:      tokenSvc,
		clientID:    result.ClientID,
		secret:      result.ClientSecret,
	}
}

func (f *tokenHandlerFixture) postForm(t *testing.T, body url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func decodeErrorBody(t *testing.T, body []byte) rfc6749ErrorResponse {
	t.Helper()
	var parsed rfc6749ErrorResponse
	require.NoError(t, json.Unmarshal(body, &parsed))
	return parsed
}

func TestTokenHandler_FormEncodedHappyPath(t *testing.T) {
	f := newTokenHandlerFixture(t)

	body := url.Values{}
	body.Set("grant_type", "client_credentials")
	body.Set("client_id", f.clientID)
	body.Set("client_secret", f.secret)
	body.Set("audience", "mihomo.sync")
	body.Set("scope", "binding.read")

	w := f.postForm(t, body)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	var resp credentials.IssuedToken
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Bearer", resp.TokenType)
	assert.Equal(t, "binding.read", resp.Scope)
	assert.Greater(t, resp.ExpiresIn, int64(0))
	require.NotEmpty(t, resp.AccessToken)

	// Decode the JWT and confirm client_id and scope claims match.
	claims := &credentials.AccessClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(resp.AccessToken, claims)
	require.NoError(t, err)
	assert.Equal(t, f.clientID, claims.ClientID)
	assert.Equal(t, "binding.read", claims.Scope)
	assert.Contains(t, claims.Audience, "mihomo.sync")
}

func TestTokenHandler_JSONBodyRejected(t *testing.T) {
	f := newTokenHandlerFixture(t)

	payload := `{"grant_type":"client_credentials","client_id":"telegram-service","client_secret":"x","audience":"mihomo.sync"}`
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	got := decodeErrorBody(t, w.Body.Bytes())
	assert.Equal(t, "invalid_request", got.Error)
}

func TestTokenHandler_FormErrors(t *testing.T) {
	cases := []struct {
		name           string
		mutate         func(values url.Values)
		wantStatus     int
		wantErrorCode  string
		wantErrorMatch string
	}{
		{
			name:          "missing grant_type",
			mutate:        func(v url.Values) { v.Del("grant_type") },
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "unsupported_grant_type",
		},
		{
			name:          "wrong grant_type",
			mutate:        func(v url.Values) { v.Set("grant_type", "password") },
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "unsupported_grant_type",
		},
		{
			name:          "missing client_id",
			mutate:        func(v url.Values) { v.Del("client_id") },
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_request",
		},
		{
			name:          "missing audience",
			mutate:        func(v url.Values) { v.Del("audience") },
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_request",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newTokenHandlerFixture(t)
			body := url.Values{}
			body.Set("grant_type", "client_credentials")
			body.Set("client_id", f.clientID)
			body.Set("client_secret", f.secret)
			body.Set("audience", "mihomo.sync")
			tc.mutate(body)

			w := f.postForm(t, body)
			require.Equal(t, tc.wantStatus, w.Code, "body=%s", w.Body.String())
			got := decodeErrorBody(t, w.Body.Bytes())
			assert.Equal(t, tc.wantErrorCode, got.Error)
		})
	}
}

func TestTokenHandler_BadClientSecretReturnsInvalidClient(t *testing.T) {
	f := newTokenHandlerFixture(t)

	body := url.Values{}
	body.Set("grant_type", "client_credentials")
	body.Set("client_id", f.clientID)
	body.Set("client_secret", "not-the-real-secret")
	body.Set("audience", "mihomo.sync")

	w := f.postForm(t, body)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	got := decodeErrorBody(t, w.Body.Bytes())
	assert.Equal(t, "invalid_client", got.Error)
}

func TestTokenHandler_UnknownClientIDReturnsInvalidClient(t *testing.T) {
	f := newTokenHandlerFixture(t)

	body := url.Values{}
	body.Set("grant_type", "client_credentials")
	body.Set("client_id", "no-such-client")
	body.Set("client_secret", "irrelevant")
	body.Set("audience", "mihomo.sync")

	w := f.postForm(t, body)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	got := decodeErrorBody(t, w.Body.Bytes())
	// RFC 6749 §5.2 deliberately collapses "unknown client" and "bad
	// secret" into the same error code to avoid leaking which one
	// failed.
	assert.Equal(t, "invalid_client", got.Error)
}

func TestTokenHandler_AudienceNotInCredentialReturnsInvalidRequest(t *testing.T) {
	f := newTokenHandlerFixture(t)

	body := url.Values{}
	body.Set("grant_type", "client_credentials")
	body.Set("client_id", f.clientID)
	body.Set("client_secret", f.secret)
	body.Set("audience", "audience-not-in-allow-list")

	w := f.postForm(t, body)
	require.Equal(t, http.StatusBadRequest, w.Code)
	got := decodeErrorBody(t, w.Body.Bytes())
	assert.Equal(t, "invalid_request", got.Error)
}

func TestTokenHandler_RequestedScopeSupersetReturnsInvalidScope(t *testing.T) {
	f := newTokenHandlerFixture(t)

	body := url.Values{}
	body.Set("grant_type", "client_credentials")
	body.Set("client_id", f.clientID)
	body.Set("client_secret", f.secret)
	body.Set("audience", "mihomo.sync")
	body.Set("scope", "binding.read binding.delete")

	w := f.postForm(t, body)
	require.Equal(t, http.StatusBadRequest, w.Code)
	got := decodeErrorBody(t, w.Body.Bytes())
	assert.Equal(t, "invalid_scope", got.Error)
}

func TestTokenHandler_DisabledCredentialReturnsInvalidClient(t *testing.T) {
	f := newTokenHandlerFixture(t)

	_, err := f.credentials.SetStatus(f.clientID, model.ServiceCredentialStatusDisabled)
	require.NoError(t, err)

	body := url.Values{}
	body.Set("grant_type", "client_credentials")
	body.Set("client_id", f.clientID)
	body.Set("client_secret", f.secret)
	body.Set("audience", "mihomo.sync")

	w := f.postForm(t, body)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	got := decodeErrorBody(t, w.Body.Bytes())
	assert.Equal(t, "invalid_client", got.Error)
}
