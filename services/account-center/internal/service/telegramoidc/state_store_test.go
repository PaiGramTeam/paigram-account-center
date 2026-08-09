package telegramoidc_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"paigram/internal/model"
	"paigram/internal/service/telegramoidc"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// cache=shared + per-test unique DSN lets concurrent goroutines (see
	// TestConsume_ConcurrentSameState) share the in-memory schema across
	// pool connections. The bare ":memory:" DSN creates a fresh DB per
	// connection, which breaks Transaction-based tests that check out a
	// second pool connection mid-flight.
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	// SQLite has no real SELECT FOR UPDATE; concurrent write txns deadlock.
	// Production runs on MySQL where clause.Locking{Strength:"UPDATE"} is
	// honored. For tests we cap the pool at 1 so concurrent Consume calls
	// serialize through the same connection — this still exercises the
	// "second consume sees ConsumedAt.Valid=true and returns
	// ErrInvalidState" path that the application code is responsible for.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.UserOAuthState{}))
	return db
}

func TestIssue_PersistsRow(t *testing.T) {
	db := newTestDB(t)
	store := telegramoidc.NewStateStore(db, zap.NewNop())

	state, err := store.Issue(context.Background(), telegramoidc.IssueInput{
		CodeVerifier: "verifier_43_char_url_safe_b64___________________",
		BotID:        "paigrambot",
		RedirectTo:   "/me/bot-identities",
		ClientIP:     "203.0.113.42",
		UserAgent:    "Mozilla/5.0 acceptance",
	})
	require.NoError(t, err)
	assert.Len(t, state, 32)

	var row model.UserOAuthState
	require.NoError(t, db.Where("state = ?", state).First(&row).Error)
	assert.Equal(t, "telegram", row.Provider)
	assert.Equal(t, "telegram_oidc", row.Purpose)
	assert.Equal(t, "verifier_43_char_url_safe_b64___________________", row.CodeVerifier)
	assert.Equal(t, "/me/bot-identities", row.RedirectTo)
	assert.Equal(t, "203.0.113.42", row.ClientIP)
	assert.Equal(t, "Mozilla/5.0 acceptance", row.UserAgent)
	assert.WithinDuration(t, time.Now().Add(10*time.Minute), row.ExpiresAt, 5*time.Second)
	assert.False(t, row.ConsumedAt.Valid)

	var meta map[string]string
	require.NoError(t, json.Unmarshal(row.Metadata, &meta))
	assert.Equal(t, "paigrambot", meta["bot_id"])
}

// testIssueInput is a fixture helper so each Consume test uses identical
// client_ip + user_agent at Issue and Consume time, which is the happy path.
const (
	testClientIP  = "203.0.113.42"
	testUserAgent = "Mozilla/5.0 acceptance"
)

func testIssueInput() telegramoidc.IssueInput {
	return telegramoidc.IssueInput{
		CodeVerifier: "v_43char_url_safe_b64___________________",
		BotID:        "paigrambot",
		RedirectTo:   "/me/bot-identities",
		ClientIP:     testClientIP,
		UserAgent:    testUserAgent,
	}
}

func testConsumeInput(state string) telegramoidc.ConsumeInput {
	return telegramoidc.ConsumeInput{
		State:     state,
		ClientIP:  testClientIP,
		UserAgent: testUserAgent,
	}
}

func TestConsume_Success(t *testing.T) {
	db := newTestDB(t)
	store := telegramoidc.NewStateStore(db, zap.NewNop())
	state, err := store.Issue(context.Background(), testIssueInput())
	require.NoError(t, err)

	rec, err := store.Consume(context.Background(), testConsumeInput(state))
	require.NoError(t, err)
	assert.Equal(t, "v_43char_url_safe_b64___________________", rec.CodeVerifier)
	assert.Equal(t, "paigrambot", rec.BotID)
	assert.Equal(t, "/me/bot-identities", rec.RedirectTo)
	assert.Equal(t, testClientIP, rec.ClientIP)
	assert.Equal(t, testUserAgent, rec.UserAgent)

	var row model.UserOAuthState
	require.NoError(t, db.Where("state = ?", state).First(&row).Error)
	assert.True(t, row.ConsumedAt.Valid)
}

func TestConsume_NotFound(t *testing.T) {
	store := telegramoidc.NewStateStore(newTestDB(t), zap.NewNop())
	_, err := store.Consume(context.Background(), testConsumeInput("ffffffffffffffffffffffffffffffff"))
	assert.ErrorIs(t, err, telegramoidc.ErrInvalidState)
}

func TestConsume_Expired(t *testing.T) {
	db := newTestDB(t)
	store := telegramoidc.NewStateStore(db, zap.NewNop())
	state, err := store.Issue(context.Background(), testIssueInput())
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.UserOAuthState{}).
		Where("state = ?", state).
		Update("expires_at", time.Now().Add(-1*time.Minute)).Error)

	_, err = store.Consume(context.Background(), testConsumeInput(state))
	assert.ErrorIs(t, err, telegramoidc.ErrInvalidState)
}

func TestConsume_AlreadyConsumed(t *testing.T) {
	db := newTestDB(t)
	store := telegramoidc.NewStateStore(db, zap.NewNop())
	state, err := store.Issue(context.Background(), testIssueInput())
	require.NoError(t, err)
	_, err = store.Consume(context.Background(), testConsumeInput(state))
	require.NoError(t, err)

	_, err = store.Consume(context.Background(), testConsumeInput(state))
	assert.ErrorIs(t, err, telegramoidc.ErrInvalidState)
}

func TestConsume_ClientIPMismatch(t *testing.T) {
	db := newTestDB(t)
	store := telegramoidc.NewStateStore(db, zap.NewNop())
	state, err := store.Issue(context.Background(), testIssueInput())
	require.NoError(t, err)

	bad := testConsumeInput(state)
	bad.ClientIP = "10.0.0.1"
	_, err = store.Consume(context.Background(), bad)
	assert.ErrorIs(t, err, telegramoidc.ErrInvalidState)

	// The state row stays un-consumed so a later legitimate consume from
	// the original client still works — defense in depth.
	rec, err := store.Consume(context.Background(), testConsumeInput(state))
	require.NoError(t, err)
	assert.Equal(t, "paigrambot", rec.BotID)
}

func TestConsume_UserAgentMismatch(t *testing.T) {
	db := newTestDB(t)
	store := telegramoidc.NewStateStore(db, zap.NewNop())
	state, err := store.Issue(context.Background(), testIssueInput())
	require.NoError(t, err)

	bad := testConsumeInput(state)
	bad.UserAgent = "curl/8.0"
	_, err = store.Consume(context.Background(), bad)
	assert.ErrorIs(t, err, telegramoidc.ErrInvalidState)
}

func TestConsume_ConcurrentSameState(t *testing.T) {
	db := newTestDB(t)
	store := telegramoidc.NewStateStore(db, zap.NewNop())
	state, err := store.Issue(context.Background(), testIssueInput())
	require.NoError(t, err)

	var wg sync.WaitGroup
	var successCount, invalidCount int
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Consume(context.Background(), testConsumeInput(state))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successCount++
			} else if errors.Is(err, telegramoidc.ErrInvalidState) {
				invalidCount++
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, successCount, "exactly one consume should win")
	assert.Equal(t, 1, invalidCount)
}
