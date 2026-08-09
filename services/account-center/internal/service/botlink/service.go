package botlink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"paigram/internal/model"
)

const (
	auditActionLinkCreated = "telegram_link_created"
	auditActionLinkRevoked = "telegram_link_revoked"
)

// UpsertLinkInput is the per-call payload for UpsertLink.
type UpsertLinkInput struct {
	BotID            string
	UserID           uint64
	ExternalUserID   string
	ExternalUsername *string
	RequestIP        string
	RequestUA        string
}

// Service is the layered-architecture Service object per account-center
// AGENTS.md §2 ("enter.go group management"). Held by the handler layer;
// owns the database transaction.
type Service struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewService(db *gorm.DB, logger *zap.Logger) *Service {
	return &Service{db: db, logger: logger}
}

// UpsertLink creates or refreshes a bot_identities row.
// Idempotent on same (user_id, bot_id, external_user_id), even after a
// prior Unlink (which soft-deletes via gorm.DeletedAt).
// Returns ErrTelegramAlreadyLinkedToOther / ErrAlreadyLinked on conflicts.
//
// Decision tree (see spec §5.2 idempotency contract):
//  1. Unscoped lookup by (bot_id, external_user_id):
//     - Found, same user, not deleted: idempotent refresh (update username
//     if changed, no audit).
//     - Found, same user, soft-deleted: revive (clear deleted_at, update
//     username, no audit). Original telegram_link_created audit is
//     preserved at the audit-log level; a previously written
//     telegram_link_revoked audit from the Unlink remains intact.
//     - Found, different user, not deleted: ErrTelegramAlreadyLinkedToOther.
//     - Found, different user, soft-deleted: hard-delete the orphan and
//     fall through to INSERT. The previous owner's revoke audit is
//     preserved at the audit-log level, so audit history is not lost.
//  2. Default-scoped lookup by (user_id, bot_id): if an active row exists
//     for this user on this bot with a DIFFERENT external_user_id,
//     reject with ErrAlreadyLinked.
//  3. INSERT new row, write telegram_link_created audit.
//  4. On UNIQUE-violation race-loss: re-lookup unscoped by
//     (bot_id, external_user_id) and apply the same same-user-vs-other
//     branching as step 1.
//
// This is a backward-compatibility wrapper that opens its own transaction
// and delegates to UpsertLinkTx. Callers that need to bundle the upsert
// into a larger transaction (e.g. handler/telegramoidc's Callback per
// spec §6.3) MUST call UpsertLinkTx directly with the outer tx.
func (s *Service) UpsertLink(ctx context.Context, in UpsertLinkInput) (*model.BotIdentity, error) {
	var result *model.BotIdentity
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = s.UpsertLinkTx(ctx, tx, in)
		return err
	})
	if txErr != nil {
		return nil, txErr
	}
	return result, nil
}

// UpsertLinkTx is the transaction-aware variant of UpsertLink. It runs
// every query — including the audit-log write — against the
// caller-supplied tx, so the row INSERT and its
// telegram_link_created audit commit (or roll back) as one unit. It MUST
// NOT call tx.Transaction(...) itself.
func (s *Service) UpsertLinkTx(ctx context.Context, tx *gorm.DB, in UpsertLinkInput) (*model.BotIdentity, error) {
	if in.BotID == "" || in.UserID == 0 || in.ExternalUserID == "" {
		return nil, errors.New("botlink: BotID, UserID, ExternalUserID required")
	}

	tx = tx.WithContext(ctx)

	// Step 1: unscoped lookup by (bot_id, external_user_id). Unscoped()
	// surfaces soft-deleted rows because production InnoDB UNIQUE keys
	// cover ALL rows; a soft-deleted row would still collide on INSERT
	// without this branch.
	var existing model.BotIdentity
	err := tx.Unscoped().
		Where("bot_id = ? AND external_user_id = ?", in.BotID, in.ExternalUserID).
		First(&existing).Error
	if err == nil {
		switch {
		case existing.UserID == in.UserID && !existing.DeletedAt.Valid:
			// Idempotent refresh — same triple, still active.
			if in.ExternalUsername != nil && (!existing.ExternalUsername.Valid || existing.ExternalUsername.String != *in.ExternalUsername) {
				if err := tx.Model(&existing).Update("external_username", *in.ExternalUsername).Error; err != nil {
					s.logger.Error("botlink: refresh username failed",
						zap.Uint64("user_id", in.UserID),
						zap.String("bot_id", in.BotID),
						zap.Error(err))
					return nil, fmt.Errorf("botlink: refresh username: %w", err)
				}
			}
			return &existing, nil
		case existing.UserID == in.UserID && existing.DeletedAt.Valid:
			// Revive — same user, previously unlinked. Treat as
			// idempotent same-triple re-link: clear deleted_at, refresh
			// username if changed, do NOT fire a new audit. The original
			// created/revoked audit pair is preserved.
			updates := map[string]any{"deleted_at": gorm.Expr("NULL")}
			if in.ExternalUsername != nil {
				updates["external_username"] = *in.ExternalUsername
			}
			if err := tx.Unscoped().Model(&existing).Updates(updates).Error; err != nil {
				s.logger.Error("botlink: revive failed",
					zap.Uint64("user_id", in.UserID),
					zap.String("bot_id", in.BotID),
					zap.Error(err))
				return nil, fmt.Errorf("botlink: revive: %w", err)
			}
			// Re-read into a fresh struct so result reflects the cleared
			// DeletedAt and any updated columns. Using a fresh value
			// avoids GORM merging the new row over the prior in-memory
			// fields (where DeletedAt.Valid was true).
			var revived model.BotIdentity
			if err := tx.Unscoped().First(&revived, existing.ID).Error; err != nil {
				return nil, fmt.Errorf("botlink: reload revived row: %w", err)
			}
			return &revived, nil
		case existing.UserID != in.UserID && !existing.DeletedAt.Valid:
			// Different user holds this (bot_id, external_user_id) pair
			// actively — surface the explicit sentinel.
			return nil, ErrTelegramAlreadyLinkedToOther
		default:
			// Different user, soft-deleted. The previous owner unlinked,
			// so the external account is available, but the orphan row
			// would still collide on the production UNIQUE index. Hard
			// delete it. The previous owner's telegram_link_revoked
			// audit row remains untouched in audit_logs.
			if err := tx.Unscoped().Delete(&existing).Error; err != nil {
				s.logger.Error("botlink: cleanup orphan failed",
					zap.Uint64("orphan_user_id", existing.UserID),
					zap.String("bot_id", in.BotID),
					zap.Error(err))
				return nil, fmt.Errorf("botlink: cleanup orphan: %w", err)
			}
			// Fall through to step 2 + step 3 (INSERT).
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		s.logger.Error("botlink: lookup by bot+ext failed",
			zap.String("bot_id", in.BotID),
			zap.Error(err))
		return nil, fmt.Errorf("botlink: lookup by bot+ext: %w", err)
	}

	// Step 2: default-scoped lookup by (user_id, bot_id). Only ACTIVE
	// rows trigger ErrAlreadyLinked — a soft-deleted row from a prior
	// Unlink on the same bot is not a blocker (the user is free to
	// link a different Telegram account to it).
	var conflict model.BotIdentity
	err = tx.Where("user_id = ? AND bot_id = ?", in.UserID, in.BotID).First(&conflict).Error
	if err == nil {
		return nil, ErrAlreadyLinked
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		s.logger.Error("botlink: lookup by user+bot failed",
			zap.Uint64("user_id", in.UserID),
			zap.String("bot_id", in.BotID),
			zap.Error(err))
		return nil, fmt.Errorf("botlink: lookup by user+bot: %w", err)
	}

	// Step 3: insert fresh row.
	row := model.BotIdentity{
		UserID:         in.UserID,
		BotID:          in.BotID,
		ExternalUserID: in.ExternalUserID,
	}
	if in.ExternalUsername != nil {
		row.ExternalUsername.String = *in.ExternalUsername
		row.ExternalUsername.Valid = true
	}
	if err := tx.Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			// Step 4: lost a race. Re-lookup unscoped to disambiguate
			// the race-loss type. Spec §5.2 requires idempotent success
			// when the winning row holds the SAME triple as our input.
			var winner model.BotIdentity
			lookupErr := tx.Unscoped().
				Where("bot_id = ? AND external_user_id = ?", in.BotID, in.ExternalUserID).
				First(&winner).Error
			if lookupErr == nil {
				if winner.UserID == in.UserID {
					s.logger.Info("botlink: race-loss same-triple, returning winner",
						zap.Uint64("user_id", in.UserID),
						zap.String("bot_id", in.BotID))
					if winner.DeletedAt.Valid {
						updates := map[string]any{"deleted_at": gorm.Expr("NULL")}
						if in.ExternalUsername != nil {
							updates["external_username"] = *in.ExternalUsername
						}
						if err := tx.Unscoped().Model(&winner).Updates(updates).Error; err != nil {
							return nil, fmt.Errorf("botlink: revive on race: %w", err)
						}
						var revived model.BotIdentity
						if err := tx.Unscoped().First(&revived, winner.ID).Error; err != nil {
							return nil, fmt.Errorf("botlink: reload race winner: %w", err)
						}
						winner = revived
					}
					return &winner, nil
				}
				s.logger.Warn("botlink: lost-race insert, winner is different user",
					zap.String("bot_id", in.BotID),
					zap.String("external_user_id", in.ExternalUserID))
				return nil, ErrTelegramAlreadyLinkedToOther
			}
			// Re-lookup found no row on the (bot_id, ext_id) pair —
			// check the (user_id, bot_id) collision case instead.
			if lookupErr := tx.Where("user_id = ? AND bot_id = ?", in.UserID, in.BotID).First(&winner).Error; lookupErr == nil {
				return nil, ErrAlreadyLinked
			}
			// Defensive fallback — should be unreachable.
			s.logger.Warn("botlink: unique violation with no resolvable winner row",
				zap.String("bot_id", in.BotID),
				zap.String("external_user_id", in.ExternalUserID))
			return nil, ErrTelegramAlreadyLinkedToOther
		}
		s.logger.Error("botlink: insert failed",
			zap.Uint64("user_id", in.UserID),
			zap.String("bot_id", in.BotID),
			zap.Error(err))
		return nil, fmt.Errorf("botlink: insert: %w", err)
	}
	if err := s.writeAudit(tx, in.UserID, auditActionLinkCreated, in); err != nil {
		return nil, err
	}
	return &row, nil
}

// ListForUser returns all bot identities owned by user_id, newest first.
func (s *Service) ListForUser(ctx context.Context, userID uint64) ([]model.BotIdentity, error) {
	var rows []model.BotIdentity
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("linked_at DESC").
		Find(&rows).Error
	if err != nil {
		s.logger.Error("botlink: list failed",
			zap.Uint64("user_id", userID),
			zap.Error(err))
		return nil, fmt.Errorf("botlink: list: %w", err)
	}
	return rows, nil
}

// Unlink deletes the row identified by (user_id, bot_id). Returns
// ErrBotIdentityNotFound if no row matched. Writes an audit log on success.
//
// Backward-compatibility wrapper around UnlinkTx; opens its own
// transaction. Callers needing to bundle the unlink with other work
// inside an outer transaction must call UnlinkTx directly.
func (s *Service) Unlink(ctx context.Context, userID uint64, botID, requestIP, requestUA string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.UnlinkTx(ctx, tx, userID, botID, requestIP, requestUA)
	})
}

// UnlinkTx is the transaction-aware variant of Unlink. It runs the
// lookup, soft-delete, and telegram_link_revoked audit write against
// the caller-supplied tx so all three commit (or roll back) as one unit.
// It MUST NOT call tx.Transaction(...) itself.
func (s *Service) UnlinkTx(ctx context.Context, tx *gorm.DB, userID uint64, botID, requestIP, requestUA string) error {
	tx = tx.WithContext(ctx)
	var row model.BotIdentity
	err := tx.Where("user_id = ? AND bot_id = ?", userID, botID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBotIdentityNotFound
		}
		s.logger.Error("botlink: lookup for delete failed",
			zap.Uint64("user_id", userID),
			zap.String("bot_id", botID),
			zap.Error(err))
		return fmt.Errorf("botlink: lookup for delete: %w", err)
	}
	if err := tx.Delete(&row).Error; err != nil {
		s.logger.Error("botlink: delete failed",
			zap.Uint64("user_id", userID),
			zap.String("bot_id", botID),
			zap.Error(err))
		return fmt.Errorf("botlink: delete: %w", err)
	}
	return s.writeAudit(tx, userID, auditActionLinkRevoked, UpsertLinkInput{
		BotID:          botID,
		UserID:         userID,
		ExternalUserID: row.ExternalUserID,
		RequestIP:      requestIP,
		RequestUA:      requestUA,
	})
}

func (s *Service) writeAudit(tx *gorm.DB, userID uint64, action string, in UpsertLinkInput) error {
	details := map[string]string{
		"bot_id":           in.BotID,
		"external_user_id": in.ExternalUserID,
	}
	if in.ExternalUsername != nil {
		details["external_username"] = *in.ExternalUsername
	}
	body, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("botlink: marshal audit: %w", err)
	}
	row := model.AuditLog{
		UserID:    userID,
		Action:    action,
		IP:        in.RequestIP,
		UserAgent: in.RequestUA,
		Details:   string(body),
	}
	if err := tx.Create(&row).Error; err != nil {
		s.logger.Error("botlink: insert audit failed",
			zap.Uint64("user_id", userID),
			zap.String("action", action),
			zap.Error(err))
		return fmt.Errorf("botlink: insert audit: %w", err)
	}
	return nil
}

// isUniqueViolation returns true if err indicates a UNIQUE-constraint
// failure. MySQL surfaces a typed *mysql.MySQLError with errno 1062
// (ER_DUP_ENTRY) — preferred to avoid false positives on error messages
// that happen to contain the literal string "1062". SQLite (via
// glebarez/sqlite, used in unit tests) lacks a typed error, so we fall
// back to a case-insensitive substring match on the well-known message.
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
