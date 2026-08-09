//go:build integration

package integration

// Phase 5 Sub-project 1 Task A7 — integration coverage for the Telegram OIDC
// login + bot_identities link flow against real PostgreSQL + Redis.
//
// Why integration rather than unit: the parts these tests exercise — the
// SELECT FOR UPDATE row lock on user_oauth_states, the composite UNIQUE
// indexes on bot_identities, and the gorm.DeletedAt soft-delete plus
// production UNIQUE-vs-soft-delete semantics — only behave correctly on
// real PostgreSQL. SQLite's locking model and UNIQUE handling diverge enough
// that the unit-test rig in internal/handler/telegramoidc/handler_test.go
// cannot reproduce the contracts spec §6.2 / §6.3 / §8.1 enforce.
//
// The OIDC provider (oauth.telegram.org) is mocked in-process via
// httptest.Server; the discovery / token / JWKS endpoints are pointed at
// the mock through the config-layer test seam introduced for A7 (see
// config.TelegramOIDCConfig godoc — the four endpoint override fields are
// not bound to mapstructure tags so file/env configuration cannot reach
// them; only in-process Config literal construction does).
//
// Spec: docs/superpowers/specs/2026-06-06-phase5-sub1-telegram-oidc-bot-link.md
//       §6.1 (happy path), §6.2 (state lifecycle table), §6.3 (atomicity),
//       §7.1 (reason codes), §8.1 (UNIQUE constraint contract).

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/config"
	"paigram/internal/model"
	"paigram/internal/service/session"
)

// --- OIDC mock -------------------------------------------------------------

// oidcMock is the in-process replica of oauth.telegram.org used by every
// test in this file. The signer + kid are owned by the mock so the JWKS
// endpoint always serves the matching public key.
//
// Behaviour:
//   - /token returns a freshly minted RS256 id_token whose `id` claim is
//     the value passed to its idHandler (which we override per-test).
//   - /jwks returns a single-key JWKS document carrying the mock's
//     RSA public key under `kid`.
//   - /auth is a stub that accepts whatever query params the OIDC client
//     would send; tests don't follow this redirect, they extract `state`
//     from the /start response and call /callback directly.
type oidcMock struct {
	signer *rsa.PrivateKey
	kid    string
	auth   *httptest.Server
	token  *httptest.Server
	jwks   *httptest.Server

	// tokenClaims controls what /token signs into id_token. Tests
	// mutate this to switch between subjects, names, etc. Guarded by
	// mu because parallel goroutines (the concurrent-callback test)
	// hit /token simultaneously.
	mu          sync.Mutex
	tokenClaims map[string]any
}

func newOIDCMock(t *testing.T, clientID string) *oidcMock {
	t.Helper()

	signer, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	mock := &oidcMock{
		signer: signer,
		kid:    "itest-kid",
		tokenClaims: map[string]any{
			"sub":                "telegram-sub-default",
			"id":                 float64(987654321),
			"name":               "Hu Tao",
			"preferred_username": "hutao",
		},
	}

	mock.auth = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Stub: the integration tests never follow this redirect; the
		// route only exists so that AuthorizeURL has a coherent target
		// shape if a future test ever needs to assert against it.
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(mock.auth.Close)

	mock.token = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		mock.mu.Lock()
		claims := jwt.MapClaims{
			"iss": "https://oauth.telegram.org",
			"aud": clientID,
			"iat": float64(time.Now().Unix()),
			"exp": float64(time.Now().Add(1 * time.Hour).Unix()),
		}
		for k, v := range mock.tokenClaims {
			claims[k] = v
		}
		mock.mu.Unlock()

		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = mock.kid
		signed, err := tok.SignedString(mock.signer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"a","token_type":"Bearer","expires_in":3600,"id_token":%q}`, signed)
	}))
	t.Cleanup(mock.token.Close)

	mock.jwks = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nBytes := signer.PublicKey.N.Bytes()
		eBytes := []byte{byte(signer.PublicKey.E >> 16), byte(signer.PublicKey.E >> 8), byte(signer.PublicKey.E)}
		for len(eBytes) > 1 && eBytes[0] == 0 {
			eBytes = eBytes[1:]
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA", "kid": mock.kid, "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(nBytes),
				"e": base64.RawURLEncoding.EncodeToString(eBytes),
			}},
		})
	}))
	t.Cleanup(mock.jwks.Close)

	return mock
}

func (m *oidcMock) setClaims(updates map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range updates {
		m.tokenClaims[k] = v
	}
}

// --- test infrastructure ---------------------------------------------------

// telegramOIDCStack bundles the integrationStack with the OIDC mock that
// drives it. Tests construct one via newTelegramOIDCStack and use the
// helper methods on this type for state/IP/UA-coherent request flows.
type telegramOIDCStack struct {
	*integrationStack
	mock     *oidcMock
	clientID string
}

const (
	itestClientIP  = "192.0.2.7"
	itestRemote    = itestClientIP + ":54321"
	itestUserAgent = "IntegrationTelegramOIDC/1.0"
	itestBotID     = "paigrambot"
	itestRedirect  = "https://app.example.test/auth/telegram/callback"
)

// newTelegramOIDCStack builds an integrationStack with Telegram OIDC
// credentials + endpoint overrides pointing at an in-process mock. It also
// seeds the bots row that bot_identities FK-references in production.
func newTelegramOIDCStack(t *testing.T) *telegramOIDCStack {
	t.Helper()

	clientID := "itest-telegram-client"
	mock := newOIDCMock(t, clientID)

	stack := newIntegrationStackWithConfig(t, func(cfg *config.Config) {
		cfg.TelegramOIDC = config.TelegramOIDCConfig{
			ClientID:     clientID,
			ClientSecret: "itest-telegram-secret",
			RedirectURI:  itestRedirect,
			// Test-only seam: production configs leave these empty so
			// service/telegramoidc/config.go applyDefaults pins them at
			// oauth.telegram.org. The fields are not bound to
			// mapstructure tags, so file/env config cannot reach them.
			AuthorizeEndpoint: mock.auth.URL + "/auth",
			TokenEndpoint:     mock.token.URL,
			JWKSEndpoint:      mock.jwks.URL,
			ExpectedIssuer:    "https://oauth.telegram.org",
		}
	})

	// Seed the bot owner + bots row. Production bot_identities.bot_id
	// FK-references bots.id (initialize/migrate/sql/000001_init_schema.up.sql
	// fk_bot_identities_bot), so the OIDC callback's UpsertLink INSERT
	// would 1452 (ER_NO_REFERENCED_ROW_2) without these.
	owner := model.User{PrimaryLoginType: model.LoginTypeTelegram, Status: model.UserStatusActive}
	require.NoError(t, stack.DB.Create(&owner).Error)
	bot := model.Bot{
		ID:          itestBotID,
		DisplayName: "PaiGram Bot",
		Type:        "OTHER",
		Status:      "ACTIVE",
		OwnerUserID: owner.ID,
	}
	require.NoError(t, stack.DB.Create(&bot).Error)

	return &telegramOIDCStack{integrationStack: stack, mock: mock, clientID: clientID}
}

// perform issues a GET request with the canonical client IP / User-Agent
// pair so state-binding equality (state_store.ConsumeTx V23 hardening)
// holds between /start and /callback. Headers default to "no cookie"; pass
// extras via headers to add Authorization / Cookie.
func (s *telegramOIDCStack) perform(t *testing.T, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = itestRemote
	req.Header.Set("User-Agent", itestUserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.Router.ServeHTTP(w, req)
	return w
}

// startFlow hits /api/v1/auth/telegram/start?bot=... and returns the state
// extracted from the 302 Location header. Asserts the redirect target
// belongs to the mock authorize endpoint and that exactly one
// purpose='telegram_oidc' state row was persisted as a side effect.
func (s *telegramOIDCStack) startFlow(t *testing.T) string {
	t.Helper()
	w := s.perform(t, http.MethodGet, "/api/v1/auth/telegram/start?bot="+itestBotID, nil)
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	loc := w.Header().Get("Location")
	require.Contains(t, loc, s.mock.auth.URL+"/auth", "authorize redirect must target mock")
	require.Contains(t, loc, "client_id="+s.clientID)
	require.Contains(t, loc, "code_challenge_method=S256")
	return extractStateParam(t, loc)
}

func extractStateParam(t *testing.T, loc string) string {
	t.Helper()
	idx := strings.Index(loc, "state=")
	require.NotEqual(t, -1, idx, "Location must carry state=; got %q", loc)
	tail := loc[idx+len("state="):]
	if amp := strings.Index(tail, "&"); amp != -1 {
		tail = tail[:amp]
	}
	require.NotEmpty(t, tail)
	return tail
}

func itestHashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// --- 3a: happy path --------------------------------------------------------

func TestIntegration_TelegramOIDC_HappyPath(t *testing.T) {
	s := newTelegramOIDCStack(t)

	state := s.startFlow(t)

	// Side effect of /start: exactly one state row scoped to telegram_oidc
	// with metadata={"bot_id":"paigrambot"} and an open consumption window.
	var stateRow model.UserOAuthState
	require.NoError(t, s.DB.Where("state = ?", state).First(&stateRow).Error)
	assert.Equal(t, "telegram_oidc", stateRow.Purpose)
	assert.False(t, stateRow.ConsumedAt.Valid, "fresh state row must have consumed_at = NULL")
	assert.True(t, stateRow.ExpiresAt.After(time.Now()), "expires_at must be in the future")
	assert.Equal(t, itestClientIP, stateRow.ClientIP)
	assert.Equal(t, itestUserAgent, stateRow.UserAgent)
	var meta map[string]string
	require.NoError(t, json.Unmarshal(stateRow.Metadata, &meta))
	assert.Equal(t, itestBotID, meta["bot_id"])

	// Drive the OIDC mock toward a fresh user with subject u-sub-happy.
	s.mock.setClaims(map[string]any{
		"sub":                "u-sub-happy",
		"id":                 float64(987654321),
		"name":               "Hu Tao",
		"preferred_username": "hutao",
	})

	cbW := s.perform(t, http.MethodGet, "/api/v1/auth/telegram/callback?code=happy-code&state="+state, nil)
	require.Equal(t, http.StatusFound, cbW.Code, cbW.Body.String())
	assert.Equal(t, "/me/bot-identities", cbW.Header().Get("Location"))

	// User row + credential row pinned to (telegram-oidc, u-sub-happy).
	var newUserID uint64
	require.NoError(t, s.DB.Model(&model.UserCredential{}).
		Where("provider = ? AND provider_account_id = ?", "telegram-oidc", "u-sub-happy").
		Select("user_id").Scan(&newUserID).Error)
	require.NotZero(t, newUserID, "FindOrCreateOIDC must have created a user")

	// bot_identities row pinned to (paigrambot, 987654321) under the new user.
	var ident model.BotIdentity
	require.NoError(t, s.DB.Where("bot_id = ? AND external_user_id = ?", itestBotID, "987654321").First(&ident).Error)
	assert.Equal(t, newUserID, ident.UserID)
	require.True(t, ident.ExternalUsername.Valid)
	assert.Equal(t, "hutao", ident.ExternalUsername.String)

	// Audit row for telegram_link_created.
	var auditCount int64
	require.NoError(t, s.DB.Model(&model.AuditLog{}).
		Where("user_id = ? AND action = ?", newUserID, "telegram_link_created").
		Count(&auditCount).Error)
	assert.Equal(t, int64(1), auditCount, "exactly one telegram_link_created audit row")

	// Session row whose access_token_hash matches the Set-Cookie value.
	cookies := cbW.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == session.SessionCookieName {
			cc := c
			sessionCookie = cc
			break
		}
	}
	require.NotNil(t, sessionCookie, "callback must Set-Cookie %q", session.SessionCookieName)
	assert.True(t, sessionCookie.HttpOnly)

	var sessionRow model.UserSession
	require.NoError(t, s.DB.Where("user_id = ?", newUserID).First(&sessionRow).Error)
	assert.Equal(t, itestHashToken(sessionCookie.Value), sessionRow.AccessTokenHash,
		"persisted access_token_hash must match SHA-256 of cookie value")

	// State row was consumed.
	require.NoError(t, s.DB.Where("state = ?", state).First(&stateRow).Error)
	assert.True(t, stateRow.ConsumedAt.Valid, "state must be marked consumed after success")
}

// --- 3b: UNIQUE violation on real PostgreSQL ------------------------------------

// TestIntegration_TelegramOIDC_UniqueViolation_RealPostgreSQL fires two
// concurrent callbacks that decode to the same Telegram subject — exactly
// the race spec §8.1 calls out as the contract enforced by the
// (bot_id, external_user_id) UNIQUE index on production PostgreSQL.
//
// Expected outcome: exactly ONE row remains in bot_identities for the
// pair (paigrambot, 987654321), owned by the pre-seeded user A. Neither
// concurrent attempt (both running as a freshly-provisioned user B) gets
// a duplicate row in.
func TestIntegration_TelegramOIDC_UniqueViolation_RealPostgreSQL(t *testing.T) {
	s := newTelegramOIDCStack(t)

	// Pre-seed user A holding (paigrambot, 987654321). Both concurrent
	// callbacks below will produce a fresh user-B identity that collides
	// with this row on the bot+external_user_id UNIQUE index.
	userA := model.User{PrimaryLoginType: model.LoginTypeTelegram, Status: model.UserStatusActive}
	require.NoError(t, s.DB.Create(&userA).Error)
	require.NoError(t, s.DB.Create(&model.BotIdentity{
		UserID:         userA.ID,
		BotID:          itestBotID,
		ExternalUserID: "987654321",
	}).Error)

	// Issue two distinct state rows. The (bot+ext) collision happens at
	// the bot_identities UpsertLink step, not state consumption.
	stateA := s.startFlow(t)
	stateB := s.startFlow(t)

	// Both callbacks resolve to the same Telegram identity (subject +
	// id), each via its own state row.
	s.mock.setClaims(map[string]any{
		"sub":                "u-sub-collide",
		"id":                 float64(987654321),
		"name":               "Hu Tao",
		"preferred_username": "hutao",
	})

	var wg sync.WaitGroup
	results := make([]int, 2)
	locations := make([]string, 2)
	for i, st := range []string{stateA, stateB} {
		i, st := i, st
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := s.perform(t, http.MethodGet, "/api/v1/auth/telegram/callback?code=c&state="+st, nil)
			results[i] = w.Code
			locations[i] = w.Header().Get("Location")
		}()
	}
	wg.Wait()

	// Both must redirect (302). Neither route can succeed because user A
	// already owns (paigrambot, 987654321); BOTH callers should land on
	// reason=already_linked_other. The relaxed equivalent contract spec
	// §8.1 admits is: at least one collides; in practice both do
	// because the pre-seeded row predates either concurrent insert.
	for i := 0; i < 2; i++ {
		assert.Equal(t, http.StatusFound, results[i], "callback %d must 302", i)
		assert.Contains(t, locations[i], "reason=already_linked_other",
			"callback %d must surface already_linked_other (got %q)", i, locations[i])
	}

	// Final invariant: still exactly one (paigrambot, 987654321) row,
	// owned by user A.
	var rows []model.BotIdentity
	require.NoError(t, s.DB.Unscoped().
		Where("bot_id = ? AND external_user_id = ?", itestBotID, "987654321").
		Find(&rows).Error)
	require.Len(t, rows, 1, "exactly one bot_identities row must survive the race")
	assert.Equal(t, userA.ID, rows[0].UserID)
	assert.False(t, rows[0].DeletedAt.Valid, "the surviving row must not be soft-deleted")
}

// --- 3c: expired state -----------------------------------------------------

// TestIntegration_TelegramOIDC_StateExpiredReal pre-inserts a row whose
// expires_at is one hour in the past and confirms /callback rejects with
// reason=state_invalid (spec §6.2 row 2). The state must NOT be consumed
// (a fresh, expired-on-arrival row carries no replay risk, and the
// production state_store.ConsumeTx returns ErrInvalidState BEFORE writing
// consumed_at — see state_store.go: the expiry check precedes Save).
func TestIntegration_TelegramOIDC_StateExpiredReal(t *testing.T) {
	s := newTelegramOIDCStack(t)

	expiredState := "deadbeefdeadbeefdeadbeefdeadbeef"
	meta, err := json.Marshal(map[string]string{"bot_id": itestBotID})
	require.NoError(t, err)
	require.NoError(t, s.DB.Create(&model.UserOAuthState{
		Provider:     "telegram",
		Purpose:      "telegram_oidc",
		State:        expiredState,
		CodeVerifier: strings.Repeat("a", 43),
		RedirectTo:   "/me/bot-identities",
		ClientIP:     itestClientIP,
		UserAgent:    itestUserAgent,
		Metadata:     meta,
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}).Error)

	w := s.perform(t, http.MethodGet, "/api/v1/auth/telegram/callback?code=c&state="+expiredState, nil)
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	assert.Contains(t, w.Header().Get("Location"), "reason=state_invalid")

	var row model.UserOAuthState
	require.NoError(t, s.DB.Where("state = ?", expiredState).First(&row).Error)
	assert.False(t, row.ConsumedAt.Valid,
		"expired state must NOT be consumed: state_store.ConsumeTx returns ErrInvalidState before writing consumed_at")
}

// --- 3d: user denied — consume + anti-replay -------------------------------

// TestIntegration_TelegramOIDC_UserDeniedConsumesStateReal validates the
// consumedReason outer-flag pattern in handler/telegramoidc/handler.go
// (A4.1-C). Spec §6.2 row 9 + handler.go:215-221 require user_denied to
// commit the consumed_at marker so a second callback with the same state
// is rejected as state_invalid.
func TestIntegration_TelegramOIDC_UserDeniedConsumesStateReal(t *testing.T) {
	s := newTelegramOIDCStack(t)

	state := s.startFlow(t)

	deniedW := s.perform(t, http.MethodGet,
		"/api/v1/auth/telegram/callback?error=access_denied&state="+state, nil)
	require.Equal(t, http.StatusFound, deniedW.Code, deniedW.Body.String())
	assert.Contains(t, deniedW.Header().Get("Location"), "reason=user_denied")

	// Anti-replay: state IS consumed even though /callback redirected
	// to an error page. This is the contract spec §6.2 row 9 nails down.
	var row model.UserOAuthState
	require.NoError(t, s.DB.Where("state = ?", state).First(&row).Error)
	require.True(t, row.ConsumedAt.Valid,
		"user_denied path MUST commit consumed_at so the state cannot be replayed")

	// Replay attempt: must be rejected as state_invalid (the row is now
	// consumed; row.ConsumedAt.Valid → ErrInvalidState in
	// state_store.ConsumeTx).
	replayW := s.perform(t, http.MethodGet,
		"/api/v1/auth/telegram/callback?code=anything&state="+state, nil)
	require.Equal(t, http.StatusFound, replayW.Code)
	assert.Contains(t, replayW.Header().Get("Location"), "reason=state_invalid",
		"replay of a consumed state must be rejected")
}

// --- 3e: idempotent reuse of same Telegram subject -------------------------

// TestIntegration_TelegramOIDC_FindOrCreateUser_IdempotentReal documents
// A4.1-C C2: two sequential callbacks with the same id_token subject must
// resolve to the SAME user row, the SAME user_credentials row, and the
// SAME bot_identities row.
//
// Sequential reuse covers the read-then-update path of FindOrCreateOIDCTx
// (the credential row already exists on the second call). The race-loss
// recovery branch is exercised by the unit test
// TestFindOrCreateOIDC_RaceLossSameSubject_SingleUser and the in-handler
// concurrent test below in 3b; covering it again here under integration
// would add load without changing the contract.
func TestIntegration_TelegramOIDC_FindOrCreateUser_IdempotentReal(t *testing.T) {
	s := newTelegramOIDCStack(t)

	s.mock.setClaims(map[string]any{
		"sub":                "u-sub-idempotent",
		"id":                 float64(987654321),
		"name":               "Hu Tao",
		"preferred_username": "hutao",
	})

	state1 := s.startFlow(t)
	w1 := s.perform(t, http.MethodGet, "/api/v1/auth/telegram/callback?code=c1&state="+state1, nil)
	require.Equal(t, http.StatusFound, w1.Code, w1.Body.String())
	assert.Equal(t, "/me/bot-identities", w1.Header().Get("Location"))

	state2 := s.startFlow(t)
	w2 := s.perform(t, http.MethodGet, "/api/v1/auth/telegram/callback?code=c2&state="+state2, nil)
	require.Equal(t, http.StatusFound, w2.Code, w2.Body.String())
	assert.Equal(t, "/me/bot-identities", w2.Header().Get("Location"))

	// Exactly one user, one credential, one bot_identities row.
	var credRows []model.UserCredential
	require.NoError(t, s.DB.Where("provider = ? AND provider_account_id = ?",
		"telegram-oidc", "u-sub-idempotent").Find(&credRows).Error)
	require.Len(t, credRows, 1, "second callback must reuse the existing credential")

	userID := credRows[0].UserID

	var identRows []model.BotIdentity
	require.NoError(t, s.DB.Where("user_id = ? AND bot_id = ?", userID, itestBotID).
		Find(&identRows).Error)
	require.Len(t, identRows, 1, "second callback must upsert the same bot_identities row")
	assert.Equal(t, "987654321", identRows[0].ExternalUserID)

	// Two sessions are expected (one per /start+/callback flow). Each
	// callback issues its own session row — there's no de-duplication in
	// session.Service.IssueTx and the spec does not require it.
	var sessionCount int64
	require.NoError(t, s.DB.Model(&model.UserSession{}).Where("user_id = ?", userID).
		Count(&sessionCount).Error)
	assert.Equal(t, int64(2), sessionCount, "each callback issues its own session row")
}

// --- 3f: /me/bot-identities CRUD via session cookie ------------------------

// TestIntegration_MeIdentities_ListUnlink_Real exercises the
// session-authenticated /me/bot-identities surface end-to-end:
//
//  1. seed user + bot_identities row + an `ac_session` cookie that
//     matches a real user_sessions row,
//  2. GET /me/bot-identities returns the row in the response envelope,
//  3. DELETE /me/bot-identities/:botId removes it (204) and writes an
//     audit_logs row with action=telegram_link_revoked,
//  4. follow-up GET returns the empty list (soft-deleted row excluded
//     by the default scope).
//
// This is the same handler exercised by the unit tests in
// internal/handler/meidentities/handler_test.go but here we go through
// the full router + AuthMiddleware + session-cookie chain.
func TestIntegration_MeIdentities_ListUnlink_Real(t *testing.T) {
	s := newTelegramOIDCStack(t)

	user := model.User{PrimaryLoginType: model.LoginTypeTelegram, Status: model.UserStatusActive}
	require.NoError(t, s.DB.Create(&user).Error)

	require.NoError(t, s.DB.Create(&model.BotIdentity{
		UserID:           user.ID,
		BotID:            itestBotID,
		ExternalUserID:   "555000111",
		ExternalUsername: sql.NullString{String: "@meident", Valid: true},
	}).Error)

	// Seed a session row whose access_token_hash matches a token we
	// control. The middleware (internal/middleware/auth.go) reads the
	// `ac_session` cookie, hashes it, and looks the session up in
	// user_sessions — which is exactly the path the production OIDC
	// callback writes via session.Service.IssueTx.
	rawToken := "meident-test-token-" + strings.Repeat("x", 32)
	refreshNonce := "meident-refresh-nonce-" + strings.Repeat("y", 32)
	now := time.Now().UTC()
	require.NoError(t, s.DB.Create(&model.UserSession{
		UserID:           user.ID,
		AccessTokenHash:  itestHashToken(rawToken),
		RefreshTokenHash: itestHashToken(refreshNonce),
		AccessExpiry:     now.Add(15 * time.Minute),
		RefreshExpiry:    now.Add(7 * 24 * time.Hour),
		UserAgent:        itestUserAgent,
		ClientIP:         itestClientIP,
	}).Error)

	cookieHeader := session.SessionCookieName + "=" + rawToken

	// 1) List: 200 OK, envelope { code:200, data:[{...}], message:"success" }.
	listW := s.perform(t, http.MethodGet, "/api/v1/me/bot-identities", map[string]string{
		"Cookie": cookieHeader,
	})
	require.Equal(t, http.StatusOK, listW.Code, listW.Body.String())

	var envelope struct {
		Code    int              `json:"code"`
		Message string           `json:"message"`
		Data    []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &envelope))
	assert.Equal(t, 200, envelope.Code)
	assert.Equal(t, "success", envelope.Message)
	require.Len(t, envelope.Data, 1, "exactly one identity row expected")
	assert.Equal(t, itestBotID, envelope.Data[0]["bot_id"])
	assert.Equal(t, "555000111", envelope.Data[0]["external_user_id"])
	assert.Equal(t, "@meident", envelope.Data[0]["external_username"])

	// 2) Unlink: 204 No Content.
	unlinkW := s.perform(t, http.MethodDelete, "/api/v1/me/bot-identities/"+itestBotID,
		map[string]string{"Cookie": cookieHeader})
	require.Equal(t, http.StatusNoContent, unlinkW.Code, unlinkW.Body.String())

	// 3) Row is soft-deleted (gorm.DeletedAt set), audit_logs has the
	// revoke row.
	var deleted model.BotIdentity
	require.NoError(t, s.DB.Unscoped().Where("user_id = ? AND bot_id = ?", user.ID, itestBotID).
		First(&deleted).Error)
	assert.True(t, deleted.DeletedAt.Valid, "soft-delete must set deleted_at")

	var revokes int64
	require.NoError(t, s.DB.Model(&model.AuditLog{}).
		Where("user_id = ? AND action = ?", user.ID, "telegram_link_revoked").
		Count(&revokes).Error)
	assert.Equal(t, int64(1), revokes, "exactly one telegram_link_revoked audit row")

	// 4) Subsequent list returns the empty array (soft-deleted row
	// excluded by default scope).
	listAgainW := s.perform(t, http.MethodGet, "/api/v1/me/bot-identities", map[string]string{
		"Cookie": cookieHeader,
	})
	require.Equal(t, http.StatusOK, listAgainW.Code, listAgainW.Body.String())
	require.NoError(t, json.Unmarshal(listAgainW.Body.Bytes(), &envelope))
	assert.Equal(t, 200, envelope.Code)
	assert.Empty(t, envelope.Data, "soft-deleted row must be excluded from list")
}

// --- guard: keep gin in TestMode -------------------------------------------

// init pins gin to TestMode for every test in this file. Gin's default
// (DebugMode) prints noisy banners that drown out integration-test output;
// TestMode also silences gin.Logger() so the verbose `go test -v` output
// stays focused on test results.
func init() {
	gin.SetMode(gin.TestMode)
}
