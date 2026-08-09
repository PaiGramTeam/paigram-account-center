package meidentities_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	return r
}

func seedLink(t *testing.T, svc *botlink.Service, userID uint64, botID, externalID string) {
	t.Helper()
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
	seedLink(t, svc, 42, "paigrambot", "ext-1")
	// Backdate paigrambot so deltabot (linked second, real-time) sorts
	// first under ORDER BY linked_at DESC. SQLite CURRENT_TIMESTAMP has
	// only second precision; this trick is borrowed from
	// botlink/service_test.go::TestListForUser_Multiple_OrderByLinkedAtDesc.
	require.NoError(t, db.Exec(
		`UPDATE bot_identities SET linked_at = '2020-01-01 00:00:00' WHERE bot_id = ?`,
		"paigrambot",
	).Error)
	seedLink(t, svc, 42, "deltabot", "ext-2")

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
	seedLink(t, svc, 42, "paigrambot", "ext-1")

	api := meidentities.NewApiGroup(svc, zap.NewNop())
	r := authedRouter(t, 42, api.Identities)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/bot-identities/paigrambot", nil)
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

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/bot-identities/never-existed", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"code":404, "data":null, "message":"not_found"}`, rec.Body.String())
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
	seedLink(t, svc, 99, "paigrambot", "ext-99")

	api := meidentities.NewApiGroup(svc, zap.NewNop())
	h := api.Identities

	// Attempt as attacker (user 42) against victim's row.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/bot-identities/paigrambot", nil)
	rec := httptest.NewRecorder()
	authedRouter(t, 42, h).ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code,
		"must be 404, NOT 403, to keep user 99's link state opaque")
	assert.JSONEq(t, `{"code":404, "data":null, "message":"not_found"}`, rec.Body.String())

	// Parallel request against a TRULY-nonexistent bot — body must be
	// byte-identical so an attacker cannot distinguish "other user's row"
	// from "row never existed".
	reqMissing := httptest.NewRequest(http.MethodDelete, "/api/v1/me/bot-identities/no-such-bot", nil)
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

func TestUnlink_AuditWritten(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	seedLink(t, svc, 42, "paigrambot", "ext-1")

	api := meidentities.NewApiGroup(svc, zap.NewNop())
	r := authedRouter(t, 42, api.Identities)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/bot-identities/paigrambot", nil)
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
