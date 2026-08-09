package user

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"paigram/internal/model"
)

// findOrCreateDBCounter generates a unique DSN suffix per setup so
// `go test -count=N` runs do not reuse a leaky in-memory DB across
// iterations.
var findOrCreateDBCounter atomic.Uint64

// newFindOrCreateTestDB returns a SQLite-in-memory *gorm.DB with the
// minimum table set required by UserService.FindOrCreateOIDC. We mirror
// the production PostgreSQL schema's UNIQUE indexes on user_credentials so
// the credential lookup behaves the same as it does in production.
//
// We do NOT use gorm.AutoMigrate on model.User / model.UserCredential
// because those structs carry database-specific column
// tags that SQLite rejects (PostgreSQL fractional-second syntax). Raw DDL
// keeps the harness portable while still exercising the real GORM
// query paths.
func newFindOrCreateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), findOrCreateDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// Suppress GORM's default record-not-found "error" logs — those
		// fire on the find-then-create fast path and are noise here.
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// SQLite has no real row-level locking; cap pool size at 1 so the
	// in-transaction Create() statements serialize through one connection
	// the way they do in production PostgreSQL.
	sqlDB.SetMaxOpenConns(1)

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

	return db
}

func TestFindOrCreateOIDC_CreatesNewUser(t *testing.T) {
	db := newFindOrCreateTestDB(t)
	svc := &UserService{db: db}

	userID, err := svc.FindOrCreateOIDC(context.Background(), "telegram-oidc", "telegram-sub-1", "Hu Tao")
	require.NoError(t, err)
	assert.NotZero(t, userID)

	var user model.User
	require.NoError(t, db.First(&user, userID).Error)
	assert.Equal(t, model.LoginTypeTelegram, user.PrimaryLoginType)
	assert.Equal(t, model.UserStatusActive, user.Status)

	var profile model.UserProfile
	require.NoError(t, db.Where("user_id = ?", userID).First(&profile).Error)
	assert.Equal(t, "Hu Tao", profile.DisplayName)

	var cred model.UserCredential
	require.NoError(t, db.Where("provider = ? AND provider_account_id = ?", "telegram-oidc", "telegram-sub-1").First(&cred).Error)
	assert.Equal(t, userID, cred.UserID)
}

func TestFindOrCreateOIDC_ReturnsExistingOnRepeatedSubject(t *testing.T) {
	db := newFindOrCreateTestDB(t)
	svc := &UserService{db: db}

	firstID, err := svc.FindOrCreateOIDC(context.Background(), "telegram-oidc", "sub-42", "Original Name")
	require.NoError(t, err)

	secondID, err := svc.FindOrCreateOIDC(context.Background(), "telegram-oidc", "sub-42", "Different Name")
	require.NoError(t, err)
	assert.Equal(t, firstID, secondID, "same (provider, subject) must yield same user")

	// display_name MUST NOT be overwritten on subsequent calls — this
	// protects users who chose a custom display name from being clobbered
	// by Telegram profile changes.
	var profile model.UserProfile
	require.NoError(t, db.Where("user_id = ?", firstID).First(&profile).Error)
	assert.Equal(t, "Original Name", profile.DisplayName)

	// Only one user row should exist.
	var count int64
	require.NoError(t, db.Model(&model.User{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestFindOrCreateOIDC_DifferentProvidersSameSubjectAreIndependent(t *testing.T) {
	db := newFindOrCreateTestDB(t)
	svc := &UserService{db: db}

	telegramID, err := svc.FindOrCreateOIDC(context.Background(), "telegram-oidc", "sub-x", "From Telegram")
	require.NoError(t, err)

	googleID, err := svc.FindOrCreateOIDC(context.Background(), "google", "sub-x", "From Google")
	require.NoError(t, err)

	assert.NotEqual(t, telegramID, googleID, "(provider, subject) is the unique key; different providers yield distinct users")
}

func TestFindOrCreateOIDC_EmptyDisplayNameFallback(t *testing.T) {
	db := newFindOrCreateTestDB(t)
	svc := &UserService{db: db}

	userID, err := svc.FindOrCreateOIDC(context.Background(), "telegram-oidc", "sub-anon", "")
	require.NoError(t, err)

	var profile model.UserProfile
	require.NoError(t, db.Where("user_id = ?", userID).First(&profile).Error)
	assert.Equal(t, "telegram-oidc_user_sub-anon", profile.DisplayName)
}

func TestFindOrCreateOIDC_RejectsEmptyInputs(t *testing.T) {
	db := newFindOrCreateTestDB(t)
	svc := &UserService{db: db}

	cases := []struct {
		name     string
		provider string
		subject  string
	}{
		{"empty provider", "", "sub-1"},
		{"empty subject", "telegram-oidc", ""},
		{"both empty", "", ""},
		{"whitespace-only provider", "   ", "sub-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.FindOrCreateOIDC(context.Background(), tc.provider, tc.subject, "Name")
			require.Error(t, err)
		})
	}
}

func TestFindOrCreateOIDC_UnknownProviderUsesOAuthLoginType(t *testing.T) {
	assert.Equal(t, model.LoginTypeOAuth, loginTypeForOIDCProvider("microsoft"))
	assert.Equal(t, model.LoginTypeTelegram, loginTypeForOIDCProvider("telegram-oidc"))
	assert.Equal(t, model.LoginTypeTelegram, loginTypeForOIDCProvider("TELEGRAM"))
	assert.Equal(t, model.LoginTypeGoogle, loginTypeForOIDCProvider("google"))
	assert.Equal(t, model.LoginTypeGithub, loginTypeForOIDCProvider("github"))
}

// TestFindOrCreateOIDC_RaceLossSameSubject_SingleUser documents the C2
// repair contract: N concurrent FindOrCreateOIDC calls for the same
// (provider, subject) must all succeed and return THE SAME user_id;
// exactly one users row is provisioned.
//
// Limitation mirrored from botlink/service_test.go:
// TestUpsertLink_ConcurrentSameTriple_IdempotentSuccess — the SQLite
// harness sets SetMaxOpenConns(1) to keep the in-transaction Create
// statements serialised through one connection, which keeps the test
// deterministic but means the actual UNIQUE-violation race-loss branch
// in FindOrCreateOIDCTx may not always be exercised here (the
// serialised second goroutine often enters the "lookup found existing"
// fast path instead). The branch is asserted at the contract level:
// concurrent callers see exactly one user and one credential. The real
// production race target is PostgreSQL where row-level locking
// permits true parallel INSERT attempts; A7 integration tests will
// exercise this path against real PostgreSQL.
func TestFindOrCreateOIDC_RaceLossSameSubject_SingleUser(t *testing.T) {
	db := newFindOrCreateTestDB(t)
	svc := &UserService{db: db}

	const n = 5
	var wg sync.WaitGroup
	ids := make([]uint64, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids[i], errs[i] = svc.FindOrCreateOIDC(
				context.Background(), "telegram-oidc", "race-sub-1", "Hu Tao",
			)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoErrorf(t, err,
			"concurrent FindOrCreateOIDC call %d must succeed (race-loss recovered)", i)
	}
	for i := 1; i < n; i++ {
		assert.Equalf(t, ids[0], ids[i],
			"call %d must return the same user_id as call 0", i)
	}

	var users int64
	require.NoError(t, db.Model(&model.User{}).Count(&users).Error)
	assert.Equal(t, int64(1), users,
		"exactly one user row must exist after N concurrent identical calls")

	var creds int64
	require.NoError(t, db.Model(&model.UserCredential{}).
		Where("provider = ? AND provider_account_id = ?", "telegram-oidc", "race-sub-1").
		Count(&creds).Error)
	assert.Equal(t, int64(1), creds,
		"exactly one credential row must exist (uniq_provider_account holds)")
}
