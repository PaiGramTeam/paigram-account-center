package meidentities_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"paigram/internal/handler/meidentities"
	"paigram/internal/middleware"
	"paigram/internal/model"
	"paigram/internal/service/botlink"
	"paigram/internal/service/entryidentity"
)

// dbCounter mirrors botlink/service_test.go: per-call shared-cache DSN
// uniqueness so `go test -count=N` reruns do not accumulate state across
// iterations.
var dbCounter atomic.Uint64

// newTestDB ports the schema-shaping pattern from botlink/service_test.go.
// We cannot AutoMigrate model.BotIdentity because its `User` / `Bot`
// associations drag database-specific timestamp defaults into the
// migration that SQLite rejects. Raw DDL mirrors the production schema in
// initialize/migrate/sql/000001_init_schema.up.sql closely enough to
// exercise the handler's read/delete paths.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), dbCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS bot_identities (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			entry_identity_ref TEXT    NOT NULL,
			entry_epoch        INTEGER NOT NULL DEFAULT 1,
			user_id           INTEGER NOT NULL,
			bot_id            TEXT    NOT NULL,
			issuer            TEXT    NOT NULL DEFAULT 'urn:paigram:entry:paigrambot',
			external_user_id  TEXT    NOT NULL,
			external_username TEXT,
			linked_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at        DATETIME
		)`).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_bot_identities_ref ON bot_identities(entry_identity_ref)`,
	).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_bot_identities_user_issuer_active ON bot_identities(user_id, issuer) WHERE deleted_at IS NULL`,
	).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_bot_identities_issuer_subject_active ON bot_identities(issuer, external_user_id) WHERE deleted_at IS NULL`,
	).Error)
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
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY, user_ref TEXT NOT NULL, owner_epoch INTEGER NOT NULL DEFAULT 1, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS bots (
		id TEXT PRIMARY KEY, entry_issuer TEXT NOT NULL, display_name TEXT NOT NULL, status TEXT NOT NULL,
		owner_user_id INTEGER NOT NULL, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS service_credentials (
		client_id TEXT PRIMARY KEY, bot_id TEXT NOT NULL, status TEXT NOT NULL, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS entry_identity_link_challenges (
		challenge_hash TEXT PRIMARY KEY, consumer TEXT NOT NULL, bot_id TEXT NOT NULL, issuer TEXT NOT NULL,
		external_subject TEXT NOT NULL, external_username TEXT, status TEXT NOT NULL, expires_at DATETIME NOT NULL,
		approved_user_id INTEGER, consumed_at DATETIME, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS entry_identity_unlink_operations (
		operation_id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, bot_id TEXT NOT NULL, entry_identity_ref TEXT NOT NULL,
		minimum_entry_epoch INTEGER NOT NULL, state TEXT NOT NULL, completed_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS platform_account_bindings (
		id INTEGER PRIMARY KEY, owner_user_id INTEGER NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS consumer_grants (
		id INTEGER PRIMARY KEY, binding_id INTEGER NOT NULL, consumer TEXT NOT NULL, pending_entry_epoch INTEGER NOT NULL DEFAULT 0
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS entry_identity_unlink_targets (
		operation_id TEXT NOT NULL, grant_id INTEGER NOT NULL, confirmed_at DATETIME, PRIMARY KEY (operation_id, grant_id)
	)`).Error)
	return db
}

// authedRouter wires the handler under a tiny stub middleware that sets
// user_id via middleware.SetUserID, matching the project-wide pattern
// used in internal/handler/me/enter_test.go (testContextWithUser).
//
// Real auth (AuthMiddleware + SessionValidation) is exercised in the
// integration tests; this harness only validates handler logic.
func authedRouter(t *testing.T, userID uint64, h *meidentities.Handler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if userID != 0 {
		r.Use(func(c *gin.Context) {
			middleware.SetUserID(c, userID)
			c.Next()
		})
	}
	r.GET("/api/v1/me/bot-identities", h.List)
	r.DELETE("/api/v1/me/bot-identities/:botId", h.Unlink)
	r.GET("/api/v1/me/bot-identities/:botId/unlink-status", h.UnlinkStatus)
	r.POST("/api/v1/me/entry-identity-links/preview", h.PreviewLink)
	r.POST("/api/v1/me/entry-identity-links/approve", h.ApproveLink)
	r.POST("/api/v1/me/entry-identity-links/cancel", h.CancelLink)
	return r
}

func TestEntryIdentityApprovalUsesBodyTokenAndRejectsReplay(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO users (id, user_ref) VALUES (42, 'user-42')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO bots (id, entry_issuer, display_name, status, owner_user_id)
		VALUES ('paigrambot', 'urn:paigram:entry:telegram', 'PaiGram', 'ACTIVE', 1)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO service_credentials (client_id, bot_id, status)
		VALUES ('telegram-service', 'paigrambot', 'active')`).Error)
	linking := entryidentity.NewService(db, zap.NewNop(), entryidentity.Config{FrontendBaseURL: "https://account.example.com"})
	started, err := linking.Start(context.Background(), entryidentity.StartInput{
		Consumer: "telegram-service", BotID: "paigrambot", ExternalSubject: "external-42", ExternalUsername: "traveler",
	})
	require.NoError(t, err)
	approvalURL, err := url.Parse(started.ApprovalURL)
	require.NoError(t, err)
	fragment, err := url.ParseQuery(approvalURL.Fragment)
	require.NoError(t, err)
	challenge := fragment.Get("challenge")
	require.NotEmpty(t, challenge)

	handler := meidentities.NewApiGroup(botlink.NewService(db, zap.NewNop()), zap.NewNop(), linking).Identities
	router := authedRouter(t, 42, handler)
	body := []byte(fmt.Sprintf(`{"challenge":%q}`, challenge))
	preview := httptest.NewRecorder()
	previewRequest := httptest.NewRequest(http.MethodPost, "/api/v1/me/entry-identity-links/preview", bytes.NewReader(body))
	previewRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(preview, previewRequest)
	require.Equal(t, http.StatusOK, preview.Code)
	assert.Equal(t, "no-store", preview.Header().Get("Cache-Control"))
	assert.NotContains(t, preview.Body.String(), challenge)
	assert.Contains(t, preview.Body.String(), `"masked_subject"`)

	approved := httptest.NewRecorder()
	approveRequest := httptest.NewRequest(http.MethodPost, "/api/v1/me/entry-identity-links/approve", bytes.NewReader(body))
	approveRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(approved, approveRequest)
	require.Equal(t, http.StatusOK, approved.Code)
	assert.NotContains(t, approved.Body.String(), challenge)

	replay := httptest.NewRecorder()
	replayRequest := httptest.NewRequest(http.MethodPost, "/api/v1/me/entry-identity-links/approve", bytes.NewReader(body))
	replayRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(replay, replayRequest)
	assert.Equal(t, http.StatusConflict, replay.Code)
}

func seedLink(t *testing.T, db *gorm.DB, svc *botlink.Service, userID uint64, botID, externalID string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT OR IGNORE INTO users (id, user_ref) VALUES (?, ?)`, userID, fmt.Sprintf("user-%d", userID)).Error)
	require.NoError(t, db.Exec(`INSERT OR IGNORE INTO bots (id, entry_issuer, display_name, status, owner_user_id)
		VALUES (?, ?, ?, 'ACTIVE', 1)`, botID, model.DefaultEntryIssuer(botID), botID).Error)
	_, err := svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID:          botID,
		UserID:         userID,
		ExternalUserID: externalID,
	})
	require.NoError(t, err)
}

func TestList_NoSession_401(t *testing.T) {
	// With userID == 0 the authedRouter skips installing the user_id
	// stub middleware, so middleware.GetUserID returns (0, false) and
	// the handler short-circuits with 401. This mirrors production
	// behavior where AuthMiddleware would reject the request before
	// reaching the handler — we cover the handler's own defensive
	// branch here.
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	api := meidentities.NewApiGroup(svc, zap.NewNop())
	r := authedRouter(t, 0, api.Identities)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/bot-identities", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	// Envelope per account-center/AGENTS.md §4 (response package).
	assert.JSONEq(t, `{"code":401, "data":null, "message":"unauthorized"}`, rec.Body.String())
}

func TestList_Empty(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	api := meidentities.NewApiGroup(svc, zap.NewNop())
	r := authedRouter(t, 42, api.Identities)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/bot-identities", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// Spec §5.4: an empty list is a JSON [] (not null, not missing key),
	// wrapped in the project response envelope. Handler initialises dtos
	// with make([]…, 0, …) so the encoder emits "[]" rather than "null".
	assert.JSONEq(t, `{"code":200, "data":[], "message":"success"}`, rec.Body.String())
}

func TestList_Multiple_OrderByLinkedAt(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	seedLink(t, db, svc, 42, "paigrambot", "ext-1")
	// Backdate paigrambot so deltabot (linked second, real-time) sorts
	// first under ORDER BY linked_at DESC. SQLite CURRENT_TIMESTAMP has
	// only second precision; this trick is borrowed from
	// botlink/service_test.go::TestListForUser_Multiple_OrderByLinkedAtDesc.
	require.NoError(t, db.Exec(
		`UPDATE bot_identities SET linked_at = '2020-01-01 00:00:00' WHERE bot_id = ?`,
		"paigrambot",
	).Error)
	seedLink(t, db, svc, 42, "deltabot", "ext-2")

	api := meidentities.NewApiGroup(svc, zap.NewNop())
	r := authedRouter(t, 42, api.Identities)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/bot-identities", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// Parse the response envelope (AGENTS.md §4) rather than a bare array.
	var env struct {
		Code    int                           `json:"code"`
		Data    []meidentities.BotIdentityDTO `json:"data"`
		Message string                        `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Equal(t, http.StatusOK, env.Code)
	assert.Equal(t, "success", env.Message)
	got := env.Data
	require.Len(t, got, 2)
	assert.Equal(t, "deltabot", got[0].BotID, "linked_at DESC: newer first")
	assert.Equal(t, "paigrambot", got[1].BotID)
	// LinkedAt must be RFC3339 — assert presence + non-empty (exact
	// timezone offset varies across OS but format must parse).
	assert.NotEmpty(t, got[0].LinkedAt)
	assert.NotEmpty(t, got[1].LinkedAt)
}

func TestUnlink_Success_204(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	seedLink(t, db, svc, 42, "paigrambot", "ext-1")

	api := meidentities.NewApiGroup(svc, zap.NewNop())
	r := authedRouter(t, 42, api.Identities)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/bot-identities/paigrambot?operation_id=00000000-0000-4000-8000-000000000011", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String(), "204 must carry no body")

	var cnt int64
	require.NoError(t, db.Model(&model.BotIdentity{}).Count(&cnt).Error)
	assert.Equal(t, int64(0), cnt, "row must be soft-deleted (excluded from default scope)")
}

func TestUnlink_NotFound_404(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	api := meidentities.NewApiGroup(svc, zap.NewNop())
	r := authedRouter(t, 42, api.Identities)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/bot-identities/never-existed?operation_id=00000000-0000-4000-8000-000000000012", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"error":{"code":"entry_identity_unlink_operation_not_found","message":"entry identity unlink operation not found"}}`, rec.Body.String())
}

// TestUnlink_OtherUserRow_404 verifies the opaque-404 invariant from
// spec §7.3: a row owned by user 99 must be indistinguishable to user
// 42 from a row that does not exist. The body must NOT change — a 403
// or any "exists but forbidden" hint would leak the row's existence to
// an attacker probing user 99's bot identities.
//
// The byte-identical comparison with a parallel TRULY-nonexistent
// request guards against future regressions (e.g. accidentally embedding
// row metadata in the 404 envelope) by requiring exact equality of the
// wire body for both opaqueness scenarios.
func TestUnlink_OtherUserRow_404(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	seedLink(t, db, svc, 99, "paigrambot", "ext-99")

	api := meidentities.NewApiGroup(svc, zap.NewNop())
	h := api.Identities

	// Attempt as attacker (user 42) against victim's row.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/bot-identities/paigrambot?operation_id=00000000-0000-4000-8000-000000000013", nil)
	rec := httptest.NewRecorder()
	authedRouter(t, 42, h).ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code,
		"must be 404, NOT 403, to keep user 99's link state opaque")
	assert.JSONEq(t, `{"error":{"code":"entry_identity_unlink_operation_not_found","message":"entry identity unlink operation not found"}}`, rec.Body.String())

	// Parallel request against a TRULY-nonexistent bot — body must be
	// byte-identical so an attacker cannot distinguish "other user's row"
	// from "row never existed".
	reqMissing := httptest.NewRequest(http.MethodDelete, "/api/v1/me/bot-identities/no-such-bot?operation_id=00000000-0000-4000-8000-000000000014", nil)
	recMissing := httptest.NewRecorder()
	authedRouter(t, 42, h).ServeHTTP(recMissing, reqMissing)
	require.Equal(t, http.StatusNotFound, recMissing.Code)
	assert.Equal(t, rec.Body.String(), recMissing.Body.String(),
		"opaque-404: other-user row must produce identical body to nonexistent row")

	// User 99's row must still exist — attacker's probe must not
	// delete real data.
	var cnt int64
	require.NoError(t, db.Model(&model.BotIdentity{}).Where("user_id = ?", uint64(99)).Count(&cnt).Error)
	assert.Equal(t, int64(1), cnt, "other user's row must be untouched")
}

func TestUnlinkStatusMissingUsesStableTerminalCode(t *testing.T) {
	db := newTestDB(t)
	linking := entryidentity.NewService(db, zap.NewNop(), entryidentity.Config{})
	handler := meidentities.NewApiGroup(botlink.NewService(db, zap.NewNop()), zap.NewNop(), linking).Identities
	router := authedRouter(t, 42, handler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/bot-identities/paigrambot/unlink-status?operation_id=00000000-0000-4000-8000-000000000019", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	assert.JSONEq(t, `{"error":{"code":"entry_identity_unlink_operation_not_found","message":"entry identity unlink operation not found"}}`, recorder.Body.String())
}

func TestUnlink_AuditWritten(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	seedLink(t, db, svc, 42, "paigrambot", "ext-1")

	api := meidentities.NewApiGroup(svc, zap.NewNop())
	r := authedRouter(t, 42, api.Identities)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/bot-identities/paigrambot?operation_id=00000000-0000-4000-8000-000000000015", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String(), "204 must carry no body")

	var audit model.AuditLog
	require.NoError(t, db.
		Where("user_id = ? AND action = ?", uint64(42), "telegram_link_revoked").
		First(&audit).Error,
		"botlink.Service.Unlink must write a telegram_link_revoked audit row in the same tx",
	)
	assert.Equal(t, uint64(42), audit.UserID)
}
