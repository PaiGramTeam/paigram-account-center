package telegramoidc_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	handlertgoidc "paigram/internal/handler/telegramoidc"
	"paigram/internal/model"
	"paigram/internal/service/botlink"
	"paigram/internal/service/session"
	telegramoidcsvc "paigram/internal/service/telegramoidc"
	"paigram/internal/service/user"
)

// dbCounter guarantees a unique SQLite DSN per setupRig call so
// `go test -count=N` re-runs do not see leftover rows from a prior
// iteration (cache=shared keeps the named in-memory DB alive across
// gorm.Open calls within the same process).
var dbCounter atomic.Uint64

// --- test rig --------------------------------------------------------------

// rigOptions captures setupRig knobs that individual tests need to override
// (e.g. swapping the token endpoint to return a 4xx so we can exercise the
// ErrTokenExchangeRejected branch without retrofitting the test rig
// afterwards). Pure data; setupRig owns interpretation.
type rigOptions struct {
	tokenHandler http.HandlerFunc // if non-nil, replaces the default 200/id_token response
}

// rigOption is the functional-options shape callers pass to setupRig.
type rigOption func(*rigOptions)

// withTokenHandler installs a custom HTTP handler on the mocked
// oauth.telegram.org/token endpoint. Used by
// TestCallback_TokenExchangeRejected to make the upstream return 400.
func withTokenHandler(h http.HandlerFunc) rigOption {
	return func(o *rigOptions) { o.tokenHandler = h }
}

type testRig struct {
	router  *gin.Engine
	db      *gorm.DB
	tokens  *httptest.Server
	jwks    *httptest.Server
	signer  *rsa.PrivateKey
	kid     string
	cleanup []func()
}

// newTestDB seeds a SQLite-in-memory DB with the table set the handler
// touches: user_oauth_states (state store), users / user_profiles /
// user_credentials (FindOrCreateOIDC), bot_identities + audit_logs
// (botlink.UpsertLink), user_sessions (session.Issue).
//
// Raw DDL rather than gorm.AutoMigrate because several models carry
// database-specific timestamp defaults that
// SQLite rejects. The DDL mirrors the production schema's UNIQUE indexes
// so race / collision paths exercise the same constraints.
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

	// user_oauth_states — matches model.UserOAuthState's table name.
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS user_oauth_states (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			provider      TEXT,
			state         TEXT,
			purpose       TEXT    NOT NULL DEFAULT 'login',
			user_id       INTEGER,
			redirect_to   TEXT,
			nonce         TEXT,
			code_verifier TEXT,
			client_ip     TEXT    NOT NULL DEFAULT '',
			user_agent    TEXT    NOT NULL DEFAULT '',
			metadata      TEXT,
			expires_at    DATETIME,
			consumed_at   DATETIME,
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_oauth_state ON user_oauth_states(state)`,
	).Error)

	// users.
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			primary_login_type TEXT    NOT NULL,
			status             TEXT    NOT NULL DEFAULT 'pending',
			primary_role_id    INTEGER,
			last_login_at      DATETIME,
			created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at         DATETIME
		)`).Error)

	// user_profiles.
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS user_profiles (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id      INTEGER NOT NULL,
			display_name TEXT    NOT NULL,
			avatar_url   TEXT,
			bio          TEXT,
			locale       TEXT    DEFAULT 'en_US',
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_user_profiles_user_id ON user_profiles(user_id)`,
	).Error)

	// user_credentials — UNIQUE (provider, provider_account_id) is the
	// canonical OIDC lookup index.
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS user_credentials (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id             INTEGER NOT NULL,
			provider            TEXT    NOT NULL,
			provider_account_id TEXT    NOT NULL,
			password_hash       TEXT,
			access_token        TEXT,
			refresh_token       TEXT,
			token_expiry        DATETIME,
			scopes              TEXT,
			last_sync_at        DATETIME,
			metadata            TEXT,
			created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_user_provider ON user_credentials(user_id, provider)`,
	).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_provider_account ON user_credentials(provider, provider_account_id)`,
	).Error)

	// bot_identities — UNIQUE indexes mirror initialize/migrate/sql
	// 000001_init_schema.up.sql to reproduce A3.1's revive/race semantics.
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS bot_identities (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id           INTEGER NOT NULL,
			bot_id            TEXT    NOT NULL,
			external_user_id  TEXT    NOT NULL,
			external_username TEXT,
			linked_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at        DATETIME
		)`).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_bot_identities_user_bot ON bot_identities(user_id, bot_id)`,
	).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_bot_identities_bot_external ON bot_identities(bot_id, external_user_id)`,
	).Error)

	// audit_logs — botlink.UpsertLink writes telegram_link_created rows.
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_logs (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id     INTEGER NOT NULL,
			action      TEXT    NOT NULL,
			resource    TEXT,
			resource_id INTEGER,
			old_value   TEXT,
			new_value   TEXT,
			ip          TEXT,
			user_agent  TEXT,
			details     TEXT,
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`).Error)

	// user_sessions — session.Service.Issue persists access/refresh hashes.
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

// defaultTokenHandler is the success-path /token mock: returns a freshly
// signed id_token carrying the standard claim set. Used unless the test
// passes withTokenHandler(...) to setupRig.
func defaultTokenHandler(signer *rsa.PrivateKey, kid string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub":                "u-sub",
			"id":                 float64(987654321),
			"iss":                "https://oauth.telegram.org",
			"aud":                "test-client",
			"iat":                float64(time.Now().Unix()),
			"exp":                float64(time.Now().Add(1 * time.Hour).Unix()),
			"name":               "Hu Tao",
			"preferred_username": "hutao",
		})
		tok.Header["kid"] = kid
		signed, err := tok.SignedString(signer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"a","token_type":"Bearer","expires_in":3600,"id_token":%q}`, signed)
	}
}

func setupRig(t *testing.T, opts ...rigOption) *testRig {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := rigOptions{}
	for _, opt := range opts {
		opt(&cfg)
	}

	db := newTestDB(t)

	signer, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kid := "test-kid"

	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nBytes := signer.PublicKey.N.Bytes()
		eBytes := []byte{byte(signer.PublicKey.E >> 16), byte(signer.PublicKey.E >> 8), byte(signer.PublicKey.E)}
		for len(eBytes) > 1 && eBytes[0] == 0 {
			eBytes = eBytes[1:]
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(nBytes),
				"e": base64.RawURLEncoding.EncodeToString(eBytes),
			}},
		})
	}))
	t.Cleanup(jwksSrv.Close)

	tokenHandler := cfg.tokenHandler
	if tokenHandler == nil {
		tokenHandler = defaultTokenHandler(signer, kid)
	}
	tokensSrv := httptest.NewServer(tokenHandler)
	t.Cleanup(tokensSrv.Close)

	logger := zap.NewNop()
	oidcClient := telegramoidcsvc.NewClient(telegramoidcsvc.Config{
		ClientID:          "test-client",
		ClientSecret:      "test-secret",
		RedirectURI:       "https://example.test/cb",
		TokenEndpoint:     tokensSrv.URL,
		JWKSEndpoint:      jwksSrv.URL,
		ExpectedIssuer:    "https://oauth.telegram.org",
		AuthorizeEndpoint: "https://oauth.telegram.org/auth",
	}, logger)

	stateStore := telegramoidcsvc.NewStateStore(db, logger)
	botlinkSvc := botlink.NewService(db, logger)
	userSvc := &user.UserService{} // construct via exported zero-value pattern below
	// UserService keeps its db field unexported; the only public
	// constructor is user.NewServiceGroup. Wire the db directly here via
	// a tiny shim so the handler exercises real user.FindOrCreateOIDC
	// against this in-memory DB.
	*userSvc = mustNewUserService(t, db)

	sessionSvc := session.NewService(db, logger, session.WithSecureCookie(false))

	h := handlertgoidc.NewHandler(db, oidcClient, stateStore, userSvc, sessionSvc, botlinkSvc, logger)
	r := gin.New()
	r.GET("/api/v1/auth/telegram/start", h.Start)
	r.GET("/api/v1/auth/telegram/callback", h.Callback)
	return &testRig{router: r, db: db, tokens: tokensSrv, jwks: jwksSrv, signer: signer, kid: kid}
}

// mustNewUserService wires a *user.UserService against db. UserService's
// db field is unexported and the only public constructor is
// user.NewServiceGroup (which returns a ServiceGroup whose UserService
// member is the value we want). Going through NewServiceGroup keeps the
// handler test honest about which constructor it depends on.
func mustNewUserService(t *testing.T, db *gorm.DB) user.UserService {
	t.Helper()
	return user.NewServiceGroup(db).UserService
}

// extractState parses the redirect Location header from /start and returns
// the embedded `state=` value. Asserts that state was present.
func extractState(t *testing.T, loc string) string {
	t.Helper()
	idx := strings.Index(loc, "state=")
	require.NotEqual(t, -1, idx, "Location must carry state=; got %q", loc)
	tail := loc[idx+len("state="):]
	if amp := strings.Index(tail, "&"); amp != -1 {
		tail = tail[:amp]
	}
	return tail
}

// --- tests -----------------------------------------------------------------

func TestStart_302Redirect(t *testing.T) {
	rig := setupRig(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/start?bot=paigrambot", nil)
	w := httptest.NewRecorder()
	rig.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	loc := w.Header().Get("Location")
	assert.Contains(t, loc, "https://oauth.telegram.org/auth")
	assert.Contains(t, loc, "code_challenge_method=S256")
	assert.Contains(t, loc, "client_id=test-client")

	var cnt int64
	rig.db.Model(&model.UserOAuthState{}).Where("purpose = ?", "telegram_oidc").Count(&cnt)
	assert.Equal(t, int64(1), cnt, "state row persisted under purpose=telegram_oidc")
}

func TestStart_MissingBotParam_400(t *testing.T) {
	rig := setupRig(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/start", nil)
	w := httptest.NewRecorder()
	rig.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// The response MUST use the unified response.Response envelope
	// (code/data/message), not the bespoke {"error":"..."} shape used
	// prior to A4.1-C. A5 was just CHANGES_REQUESTED for the same
	// anti-pattern; this assertion locks the envelope contract in.
	assert.JSONEq(t,
		`{"code":400, "data":null, "message":"bot query parameter required"}`,
		w.Body.String(),
	)
}

func TestCallback_HappyPath_CreatesAllState(t *testing.T) {
	rig := setupRig(t)
	startReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/start?bot=paigrambot", nil)
	startW := httptest.NewRecorder()
	rig.router.ServeHTTP(startW, startReq)
	state := extractState(t, startW.Header().Get("Location"))

	cbReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/callback?code=c&state="+state, nil)
	cbW := httptest.NewRecorder()
	rig.router.ServeHTTP(cbW, cbReq)

	assert.Equal(t, http.StatusFound, cbW.Code)
	assert.Equal(t, "/me/bot-identities", cbW.Header().Get("Location"))

	// User row.
	var users int64
	require.NoError(t, rig.db.Model(&model.User{}).Count(&users).Error)
	assert.Equal(t, int64(1), users, "exactly one user provisioned")

	// Credential row pinned to (telegram-oidc, u-sub).
	var creds int64
	require.NoError(t, rig.db.Model(&model.UserCredential{}).
		Where("provider = ? AND provider_account_id = ?", "telegram-oidc", "u-sub").
		Count(&creds).Error)
	assert.Equal(t, int64(1), creds)

	// bot_identities row pinned to (paigrambot, "987654321").
	var ids int64
	require.NoError(t, rig.db.Model(&model.BotIdentity{}).
		Where("bot_id = ? AND external_user_id = ?", "paigrambot", "987654321").
		Count(&ids).Error)
	assert.Equal(t, int64(1), ids)

	// Session row + Set-Cookie.
	var sessions int64
	require.NoError(t, rig.db.Model(&model.UserSession{}).Count(&sessions).Error)
	assert.Equal(t, int64(1), sessions)
	cookies := cbW.Result().Cookies()
	require.NotEmpty(t, cookies, "callback must Set-Cookie on success")
	var found bool
	for _, c := range cookies {
		if c.Name == session.SessionCookieName {
			found = true
			assert.True(t, c.HttpOnly)
			break
		}
	}
	assert.True(t, found, "session cookie %q must be present", session.SessionCookieName)
}

func TestCallback_StateInvalid_302WithReason(t *testing.T) {
	rig := setupRig(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/callback?code=c&state=ffffffffffffffffffffffffffffffff", nil)
	w := httptest.NewRecorder()
	rig.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "reason=state_invalid")
}

func TestCallback_UserDenied(t *testing.T) {
	rig := setupRig(t)
	startW := httptest.NewRecorder()
	rig.router.ServeHTTP(startW, httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/start?bot=paigrambot", nil))
	state := extractState(t, startW.Header().Get("Location"))

	// First denial: state consumed, reason=user_denied returned.
	w := httptest.NewRecorder()
	rig.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/callback?error=access_denied&state="+state, nil))
	assert.Contains(t, w.Header().Get("Location"), "reason=user_denied")

	// Replay attempt: state must already be consumed → reason=state_invalid.
	w2 := httptest.NewRecorder()
	rig.router.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/callback?error=access_denied&state="+state, nil))
	assert.Contains(t, w2.Header().Get("Location"), "reason=state_invalid")
}

// TestCallback_TokenExchangeRejected exercises the ErrTokenExchangeRejected
// branch by swapping the mocked /token endpoint to return 400 BEFORE
// constructing the OIDC client. The variadic-option pattern on setupRig
// makes this clean (no rig-rebuild, no race-prone Close-then-replace).
func TestCallback_TokenExchangeRejected(t *testing.T) {
	rig := setupRig(t, withTokenHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))

	startW := httptest.NewRecorder()
	rig.router.ServeHTTP(startW, httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/start?bot=paigrambot", nil))
	state := extractState(t, startW.Header().Get("Location"))

	cbW := httptest.NewRecorder()
	rig.router.ServeHTTP(cbW, httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/callback?code=c&state="+state, nil))
	assert.Equal(t, http.StatusFound, cbW.Code)
	assert.Contains(t, cbW.Header().Get("Location"), "reason=token_invalid")
}

func TestCallback_AlreadyLinkedOther(t *testing.T) {
	rig := setupRig(t)

	// Pre-seed: user 99 already owns (paigrambot, 987654321).
	require.NoError(t, rig.db.Exec(
		`INSERT INTO users (id, primary_login_type, status) VALUES (?, ?, ?)`,
		99, model.LoginTypeTelegram, model.UserStatusActive,
	).Error)
	require.NoError(t, rig.db.Exec(
		`INSERT INTO bot_identities (user_id, bot_id, external_user_id) VALUES (?, ?, ?)`,
		99, "paigrambot", "987654321",
	).Error)

	startW := httptest.NewRecorder()
	rig.router.ServeHTTP(startW, httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/start?bot=paigrambot", nil))
	state := extractState(t, startW.Header().Get("Location"))

	w := httptest.NewRecorder()
	rig.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/callback?code=c&state="+state, nil))
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "reason=already_linked_other")
}

// TestCallback_RaceDoubleConsume verifies the state row's SELECT FOR
// UPDATE serialisation: when two callbacks race against the same state,
// one wins (302 to redirect_to) and the loser sees reason=state_invalid.
//
// SQLite SetMaxOpenConns(1) (see newTestDB) serialises both goroutines on
// the same connection — the consume tx commits or rolls back atomically,
// matching production PostgreSQL row-lock semantics.
func TestCallback_RaceDoubleConsume(t *testing.T) {
	rig := setupRig(t)
	startW := httptest.NewRecorder()
	rig.router.ServeHTTP(startW, httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/start?bot=paigrambot", nil))
	state := extractState(t, startW.Header().Get("Location"))

	var wg sync.WaitGroup
	locations := make([]string, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			rig.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/callback?code=c&state="+state, nil))
			locations[i] = w.Header().Get("Location")
		}()
	}
	wg.Wait()

	// Exactly one of the two callers must reach /me/bot-identities; the
	// other must land on /auth/telegram/error?reason=state_invalid.
	var successCount, invalidCount int
	for _, loc := range locations {
		switch {
		case loc == "/me/bot-identities":
			successCount++
		case strings.Contains(loc, "reason=state_invalid"):
			invalidCount++
		}
	}
	assert.Equal(t, 1, successCount, "exactly one race winner")
	assert.Equal(t, 1, invalidCount, "exactly one race loser sees state_invalid")
}
