package botlink_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"paigram/internal/model"
	"paigram/internal/service/botlink"
)

// dbCounter generates a process-unique suffix for each newTestDB call so
// repeated `go test -count=N` invocations (which reuse t.Name()) do not
// land in the same shared-cache in-memory DB and accumulate state across
// iterations. Without this, soft-deleted rows from iteration k leak into
// iteration k+1 and break assertions about audit-log counts.
var dbCounter atomic.Uint64

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Per A1's state_store_test.go convention: a per-call shared-cache DSN
	// keeps the in-memory schema visible across pool connections used by
	// Transaction(), while SetMaxOpenConns(1) serializes writers to avoid
	// SQLite's lack of true row-level locks. The dbCounter suffix makes
	// the DSN unique per invocation (see comment on dbCounter).
	//
	// We do NOT call AutoMigrate on model.BotIdentity / model.AuditLog
	// because GORM follows the BotIdentity struct's `User User` and
	// `Bot Bot` association fields and tries to also migrate model.User
	// and model.Bot. Their schema includes database-specific types and
	// defaults, so this focused SQLite harness uses raw
	// CREATE TABLE statements that mirror the production schema
	// (initialize/migrate/sql/000001_init_schema.up.sql) using
	// SQLite-portable types. Production PostgreSQL schema is verified by the
	// integration tests; this harness only validates the
	// service's query/transaction logic.
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), dbCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	// bot_identities has the same two whole-table UNIQUE constraints as the
	// production schema. Both
	// production UNIQUE keys cover ALL rows (no `WHERE deleted_at IS NULL`
	// partial-index filter).
	// We mirror that so the test harness reproduces the soft-delete
	// collision behavior exercised by the revive path in UpsertLink.
	//
	// CREATE TABLE/INDEX use IF NOT EXISTS so the harness is safe under
	// `go test -count=N` reruns. SQLite's cache=shared DSN keeps the
	// named in-memory DB alive across `gorm.Open` calls within the same
	// process, so the second iteration would otherwise fail with "table
	// bot_identities already exists".
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

	// audit_logs — mirrors initialize/migrate/sql/000001_init_schema.up.sql
	// table audit_logs. Service only writes user_id, action, ip,
	// user_agent, details — other columns are nullable in production.
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

func TestUpsertLink_NewRow(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())

	username := "hutao"
	row, err := svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID:            "paigrambot",
		UserID:           42,
		ExternalUserID:   "987654321",
		ExternalUsername: &username,
		RequestIP:        "127.0.0.1",
		RequestUA:        "test-agent",
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(42), row.UserID)
	assert.Equal(t, "987654321", row.ExternalUserID)
	assert.Equal(t, "paigrambot", row.BotID)

	var audit model.AuditLog
	require.NoError(t, db.Where("user_id = ? AND action = ?", uint64(42), "telegram_link_created").First(&audit).Error)
	var details map[string]string
	require.NoError(t, json.Unmarshal([]byte(audit.Details), &details))
	assert.Equal(t, "paigrambot", details["bot_id"])
	assert.Equal(t, "987654321", details["external_user_id"])
	assert.Equal(t, "hutao", details["external_username"])
}

func TestUpsertLink_SameUserSameTelegram_Idempotent(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	in := botlink.UpsertLinkInput{BotID: "b", UserID: 1, ExternalUserID: "e"}

	_, err := svc.UpsertLink(context.Background(), in)
	require.NoError(t, err)
	_, err = svc.UpsertLink(context.Background(), in)
	require.NoError(t, err)

	var cnt int64
	db.Model(&model.BotIdentity{}).Count(&cnt)
	assert.Equal(t, int64(1), cnt)

	var audits int64
	db.Model(&model.AuditLog{}).Where("action = ?", "telegram_link_created").Count(&audits)
	assert.Equal(t, int64(1), audits, "audit should fire only on insert")
}

func TestUpsertLink_RefreshesUsername(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	oldName, newName := "old", "new"
	_, err := svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID: "b", UserID: 1, ExternalUserID: "e", ExternalUsername: &oldName,
	})
	require.NoError(t, err)
	_, err = svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID: "b", UserID: 1, ExternalUserID: "e", ExternalUsername: &newName,
	})
	require.NoError(t, err)
	var row model.BotIdentity
	require.NoError(t, db.First(&row).Error)
	assert.Equal(t, "new", row.ExternalUsername.String)
}

func TestUpsertLink_DifferentUserSameTelegram_Rejects(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	_, err := svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID: "b", UserID: 1, ExternalUserID: "e",
	})
	require.NoError(t, err)
	_, err = svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID: "b", UserID: 2, ExternalUserID: "e",
	})
	assert.ErrorIs(t, err, botlink.ErrTelegramAlreadyLinkedToOther)
}

func TestUpsertLink_SameUserDifferentTelegram_Rejects(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	_, err := svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID: "b", UserID: 1, ExternalUserID: "e1",
	})
	require.NoError(t, err)
	_, err = svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID: "b", UserID: 1, ExternalUserID: "e2",
	})
	assert.ErrorIs(t, err, botlink.ErrAlreadyLinked)
}

func TestListForUser_Empty(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	rows, err := svc.ListForUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestListForUser_Multiple_OrderByLinkedAtDesc(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	_, err := svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID: "paigrambot", UserID: 1, ExternalUserID: "e1",
	})
	require.NoError(t, err)
	// Force a distinct linked_at on row 1 so the ORDER BY linked_at DESC
	// outcome is deterministic. SQLite's CURRENT_TIMESTAMP has only
	// second precision and two back-to-back inserts can share a tick.
	// We use a string literal in ISO-8601 form (what SQLite stores
	// natively for DATETIME) so the comparison is lexicographic and
	// matches CURRENT_TIMESTAMP output.
	require.NoError(t, db.Exec(
		`UPDATE bot_identities SET linked_at = ? WHERE bot_id = ?`,
		time.Now().Add(-1*time.Minute).UTC().Format("2006-01-02 15:04:05"), "paigrambot",
	).Error)
	_, err = svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID: "deltabot", UserID: 1, ExternalUserID: "e2",
	})
	require.NoError(t, err)
	rows, err := svc.ListForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	// deltabot was linked second (and paigrambot was backdated) → comes first
	assert.Equal(t, "deltabot", rows[0].BotID,
		"row0 linked_at=%v row1 linked_at=%v", rows[0].LinkedAt, rows[1].LinkedAt)
}

func TestUnlink_Success_WritesAudit(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	_, err := svc.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID: "b", UserID: 1, ExternalUserID: "e",
	})
	require.NoError(t, err)
	require.NoError(t, svc.Unlink(context.Background(), 1, "b", "1.1.1.1", "ua"))
	var cnt int64
	db.Model(&model.BotIdentity{}).Count(&cnt)
	assert.Equal(t, int64(0), cnt)
	var audits int64
	db.Model(&model.AuditLog{}).Where("action = ?", "telegram_link_revoked").Count(&audits)
	assert.Equal(t, int64(1), audits)
}

func TestUnlink_NotFound(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	err := svc.Unlink(context.Background(), 1, "b", "", "")
	assert.ErrorIs(t, err, botlink.ErrBotIdentityNotFound)
}

// TestUpsertLink_AfterUnlink_RevivesIdempotently exercises the
// soft-delete revive path: a user unlinks, then UpsertLinks the same
// (bot_id, external_user_id) triple. The row's deleted_at must clear,
// no new telegram_link_created audit must fire (spec §5.2 — revive is
// treated as idempotent same-triple re-link), and the original
// created + revoked audit pair must remain intact.
func TestUpsertLink_AfterUnlink_RevivesIdempotently(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	ctx := context.Background()

	_, err := svc.UpsertLink(ctx, botlink.UpsertLinkInput{
		BotID: "b", UserID: 1, ExternalUserID: "e",
	})
	require.NoError(t, err)

	require.NoError(t, svc.Unlink(ctx, 1, "b", "1.1.1.1", "ua"))

	row, err := svc.UpsertLink(ctx, botlink.UpsertLinkInput{
		BotID: "b", UserID: 1, ExternalUserID: "e",
	})
	require.NoError(t, err, "re-link of same triple after unlink must succeed (revive)")
	require.NotNil(t, row)
	assert.False(t, row.DeletedAt.Valid, "revived row must have deleted_at cleared")

	// Audit log accounting: 1 created (original UpsertLink), 1 revoked
	// (Unlink), and 0 new audits on revive.
	var created, revoked int64
	db.Model(&model.AuditLog{}).Where("action = ?", "telegram_link_created").Count(&created)
	db.Model(&model.AuditLog{}).Where("action = ?", "telegram_link_revoked").Count(&revoked)
	assert.Equal(t, int64(1), created, "revive must NOT fire a new telegram_link_created audit")
	assert.Equal(t, int64(1), revoked, "Unlink's revoke audit must remain")

	// Exactly 1 row in the table (the revived row).
	var cnt int64
	db.Model(&model.BotIdentity{}).Count(&cnt)
	assert.Equal(t, int64(1), cnt)
}

// TestUpsertLink_ConcurrentSameTriple_IdempotentSuccess exercises the
// race-loss path: N concurrent UpsertLink calls with identical triple.
// Per spec §5.2, ALL calls must succeed (the winner's INSERT writes the
// audit; losers idempotent-return the winner's row via the post-race
// unscoped re-lookup branch).
//
// Limitation: this harness sets SetMaxOpenConns(1) per A1 convention,
// which serializes goroutines at the connection level. With serial
// execution, the UNIQUE-violation race-loss branch in UpsertLink is not
// always exercised — call k > 1 may enter the step-1 same-user-active
// idempotent-refresh branch instead. The test still has value as an
// end-to-end "N concurrent calls with same triple all succeed without
// error, exactly 1 row, exactly 1 audit" smoke. The race-loss branch
// itself is exercised by production PostgreSQL where row-level locking
// permits true parallel INSERT attempts.
func TestUpsertLink_ConcurrentSameTriple_IdempotentSuccess(t *testing.T) {
	db := newTestDB(t)
	svc := botlink.NewService(db, zap.NewNop())
	ctx := context.Background()
	in := botlink.UpsertLinkInput{BotID: "b", UserID: 1, ExternalUserID: "e"}

	const n = 5
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.UpsertLink(ctx, in)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "concurrent UpsertLink call %d must succeed (idempotent same-triple)", i)
	}

	var cnt int64
	db.Model(&model.BotIdentity{}).Count(&cnt)
	assert.Equal(t, int64(1), cnt, "exactly 1 row must exist after N idempotent concurrent calls")

	var audits int64
	db.Model(&model.AuditLog{}).Where("action = ?", "telegram_link_created").Count(&audits)
	assert.Equal(t, int64(1), audits, "exactly 1 audit must fire (on the original INSERT only)")
}
