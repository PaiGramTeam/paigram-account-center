package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"paigram/internal/model"
)

// FindOrCreateOIDC returns the user ID for the given (provider, subject)
// pair, creating fresh users + user_credentials rows on first sight.
//
// Behavior:
//   - If a user_credentials row exists for (provider, providerAccountID), its
//     user_id is returned. displayName is ignored (we never overwrite the
//     stored profile.display_name on subsequent logins).
//   - Otherwise a new user (status=active, primary_login_type derived from
//     provider), profile (display_name=displayName, falling back to
//     "<provider>_user_<subject>" when empty), and user_credentials row are
//     created atomically. The user's ID is returned.
//
// provider is the credential provider key (e.g. "telegram-oidc"). subject
// is the OIDC `sub` claim (or any other stable, provider-issued identifier).
// displayName seeds users.profile.display_name only on first insert and is
// trimmed; empty falls back to a deterministic synthetic name.
//
// This is the find-or-create primitive consumed by handler/telegramoidc per
// docs/superpowers/specs/2026-06-06-phase5-sub1-telegram-oidc-bot-link.md §5.3.
// It is provider-agnostic: any OIDC-style flow that has already verified its
// id_token may use it to map (provider, sub) → users.id.
//
// Caller-side logging: this method returns wrapped errors and does NOT log.
// Callers own their own structured logging — handler/telegramoidc writes
// zap.Error fields at the call site.
//
// This is a backward-compatibility wrapper that opens its own transaction
// and delegates to FindOrCreateOIDCTx. Callers that need to bundle the
// find-or-create into a larger transaction (e.g. handler/telegramoidc's
// Callback per spec §6.3) MUST call FindOrCreateOIDCTx with the outer tx.
func (s *UserService) FindOrCreateOIDC(ctx context.Context, provider, subject, displayName string) (uint64, error) {
	var resultID uint64
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		resultID, err = s.FindOrCreateOIDCTx(ctx, tx, provider, subject, displayName)
		return err
	})
	if txErr != nil {
		return 0, txErr
	}
	return resultID, nil
}

// FindOrCreateOIDCTx is the transaction-aware variant of FindOrCreateOIDC.
// It runs every query against the caller-supplied tx and MUST NOT call
// tx.Transaction(...) itself.
//
// Race-loss recovery: if two concurrent OIDC callbacks for the same
// (provider, subject) both pass the credential lookup, both will attempt
// INSERT and the loser hits the uniq_provider_account UNIQUE index. We
// detect that via isUniqueViolation and re-lookup the credential under
// the same tx; the winning user_id is returned so legitimate
// double-clicks (or load-balanced retries) succeed idempotently rather
// than surfacing reason=internal to the user. Production MySQL InnoDB is
// the real race target; the SQLite test harness serializes via
// SetMaxOpenConns(1) but the recovery code path is still asserted by
// TestFindOrCreateOIDC_RaceLossSameSubject_SingleUser.
func (s *UserService) FindOrCreateOIDCTx(ctx context.Context, tx *gorm.DB, provider, subject, displayName string) (uint64, error) {
	provider = strings.TrimSpace(provider)
	subject = strings.TrimSpace(subject)
	if provider == "" || subject == "" {
		return 0, errors.New("user: provider and subject required")
	}

	tx = tx.WithContext(ctx)

	// Find existing credential first — the (provider, provider_account_id)
	// pair carries a UNIQUE index in production (uniq_provider_account
	// on user_credentials), so this is the canonical lookup.
	var existing model.UserCredential
	err := tx.Where("provider = ? AND provider_account_id = ?", provider, subject).
		First(&existing).Error
	if err == nil {
		return existing.UserID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, fmt.Errorf("user: lookup credential: %w", err)
	}

	// First sight — create user, profile, and credential row atomically.
	user := model.User{
		PrimaryLoginType: loginTypeForOIDCProvider(provider),
		Status:           model.UserStatusActive,
	}
	if err := tx.Create(&user).Error; err != nil {
		return 0, fmt.Errorf("user: create user: %w", err)
	}

	display := strings.TrimSpace(displayName)
	if display == "" {
		display = fmt.Sprintf("%s_user_%s", provider, subject)
	}
	profile := model.UserProfile{
		UserID:      user.ID,
		DisplayName: display,
		Locale:      "en_US",
	}
	if err := tx.Create(&profile).Error; err != nil {
		return 0, fmt.Errorf("user: create profile: %w", err)
	}

	credential := model.UserCredential{
		UserID:            user.ID,
		Provider:          provider,
		ProviderAccountID: subject,
	}
	if err := tx.Create(&credential).Error; err != nil {
		if isUniqueViolation(err) {
			// Race loss: a concurrent OIDC callback for the same
			// (provider, subject) won the INSERT race. Re-lookup the
			// credential under the same tx and return that winner's
			// user_id so the duplicate caller succeeds idempotently
			// instead of bubbling up a reason=internal redirect.
			//
			// Note: the user + profile rows this loser just created
			// become orphans. Cleaning them up here would require
			// either (a) deleting via the outer tx (safe but tightly
			// couples to the wrapping transaction's commit) or
			// (b) leaving them and accepting the orphan. We accept
			// the orphan because the legitimate-double-click rate is
			// near zero in production and a stranded user row with
			// no credential is harmless (no login path can reach it).
			// Mirror of botlink/service.go:178-211 race-loss pattern.
			var winner model.UserCredential
			if lookupErr := tx.Where("provider = ? AND provider_account_id = ?", provider, subject).
				First(&winner).Error; lookupErr == nil {
				return winner.UserID, nil
			}
			return 0, fmt.Errorf("user: create credential (race-loss, winner not found): %w", err)
		}
		return 0, fmt.Errorf("user: create credential: %w", err)
	}

	return user.ID, nil
}

// loginTypeForOIDCProvider maps an OIDC provider key to the corresponding
// model.LoginType. Unknown providers fall back to LoginTypeOAuth so the
// PrimaryLoginType column always carries a valid enum value.
func loginTypeForOIDCProvider(provider string) model.LoginType {
	switch strings.ToLower(provider) {
	case "telegram", "telegram-oidc":
		return model.LoginTypeTelegram
	case "google":
		return model.LoginTypeGoogle
	case "github":
		return model.LoginTypeGithub
	default:
		return model.LoginTypeOAuth
	}
}

// isUniqueViolation returns true if err indicates a UNIQUE-constraint
// failure. MySQL surfaces a typed *mysql.MySQLError with errno 1062
// (ER_DUP_ENTRY) — preferred over text matching to avoid false positives
// on error messages that happen to contain the literal "1062". SQLite
// (via glebarez/sqlite, used in unit tests) lacks a typed error, so we
// fall back to a case-insensitive substring match on the well-known
// message. Mirrors botlink/service.go's isUniqueViolation; kept local
// rather than refactored into a shared internal/db/errors helper to
// limit blast radius of this repair commit.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") || strings.Contains(msg, "unique")
}
