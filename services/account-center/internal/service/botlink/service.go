package botlink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"paigram/internal/model"
)

const (
	auditActionLinkCreated = "telegram_link_created"
	auditActionLinkRevoked = "telegram_link_revoked"
)

// UpsertLinkInput is the per-call payload for UpsertLink.
type UpsertLinkInput struct {
	BotID            string
	Issuer           string
	UserID           uint64
	ExternalUserID   string
	ExternalUsername *string
	RequestIP        string
	RequestUA        string
}

// Service owns bot identity persistence and audit transactions.
type Service struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewService(db *gorm.DB, logger *zap.Logger) *Service {
	return &Service{db: db, logger: logger}
}

// UpsertLink creates or refreshes a bot_identities row.
// It is idempotent for the same active (user, issuer, external subject).
// Returns ErrTelegramAlreadyLinkedToOther / ErrAlreadyLinked on conflicts.
//
// Active ownership is checked before historical rows. A matching historical
// row owned by the same user is preserved as an immutable tombstone after
// unlink. Unique-race losers re-read the active winner and return the same
// conflict semantics.
//
// Callers that need a larger atomic operation must call UpsertLinkTx.
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
	var user model.User
	if err := tx.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", in.UserID).Take(&user).Error; err != nil {
		return nil, fmt.Errorf("botlink: lock user: %w", err)
	}
	issuer, err := resolveRegisteredIssuer(tx, in.BotID, in.Issuer)
	if err != nil {
		return nil, err
	}
	in.Issuer = issuer
	var pendingUnlinks int64
	if err := tx.Model(&model.EntryIdentityUnlinkOperation{}).
		Where("user_id = ? AND bot_id = ? AND state = ?", in.UserID, in.BotID, model.EntryIdentityUnlinkPending).
		Where(`EXISTS (
			SELECT 1 FROM entry_identity_unlink_targets
			WHERE entry_identity_unlink_targets.operation_id = entry_identity_unlink_operations.operation_id
			AND entry_identity_unlink_targets.confirmed_at IS NULL
		)`).
		Count(&pendingUnlinks).Error; err != nil {
		return nil, fmt.Errorf("botlink: check pending unlink: %w", err)
	}
	if pendingUnlinks > 0 {
		return nil, ErrUnlinkPending
	}

	// Step 1: active ownership is unique within the registered issuer.
	var existing model.BotIdentity
	err = tx.
		Where("issuer = ? AND external_user_id = ?", issuer, in.ExternalUserID).
		First(&existing).Error
	if err == nil {
		if existing.UserID == in.UserID {
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
		}
		return nil, ErrTelegramAlreadyLinkedToOther
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		s.logger.Error("botlink: lookup by bot+ext failed",
			zap.String("bot_id", in.BotID),
			zap.Error(err))
		return nil, fmt.Errorf("botlink: lookup by bot+ext: %w", err)
	}

	// Step 2: one user can hold only one active subject per issuer.
	var conflict model.BotIdentity
	err = tx.Where("user_id = ? AND issuer = ?", in.UserID, issuer).First(&conflict).Error
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

	// Step 3: insert a fresh active row. Historical rows remain immutable.
	var entryEpoch uint64
	if err := tx.Unscoped().Model(&model.BotIdentity{}).Where("user_id = ?", in.UserID).
		Select("COALESCE(MAX(entry_epoch), 1)").Scan(&entryEpoch).Error; err != nil {
		return nil, fmt.Errorf("botlink: derive entry epoch: %w", err)
	}
	row := model.BotIdentity{
		UserID:         in.UserID,
		BotID:          in.BotID,
		Issuer:         issuer,
		EntryEpoch:     entryEpoch,
		ExternalUserID: in.ExternalUserID,
	}
	if in.ExternalUsername != nil {
		row.ExternalUsername.String = *in.ExternalUsername
		row.ExternalUsername.Valid = true
	}
	insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if insert.Error != nil {
		s.logger.Error("botlink: insert failed",
			zap.Uint64("user_id", in.UserID),
			zap.String("bot_id", in.BotID),
			zap.Error(insert.Error))
		return nil, fmt.Errorf("botlink: insert: %w", insert.Error)
	}
	if insert.RowsAffected == 0 {
		var winner model.BotIdentity
		if err := tx.Where("issuer = ? AND external_user_id = ?", issuer, in.ExternalUserID).First(&winner).Error; err == nil {
			if winner.UserID == in.UserID {
				return &winner, nil
			}
			return nil, ErrTelegramAlreadyLinkedToOther
		}
		if err := tx.Where("user_id = ? AND issuer = ?", in.UserID, issuer).First(&winner).Error; err == nil {
			return nil, ErrAlreadyLinked
		}
		return nil, errors.New("botlink: conflict winner could not be resolved")
	}
	if err := s.writeAudit(tx, in.UserID, auditActionLinkCreated, in); err != nil {
		return nil, err
	}
	return &row, nil
}

func resolveRegisteredIssuer(tx *gorm.DB, botID, presentedIssuer string) (string, error) {
	var bot model.Bot
	if err := tx.Select("entry_issuer").Where("id = ? AND status = ?", botID, "ACTIVE").Take(&bot).Error; err != nil {
		return "", fmt.Errorf("botlink: load registered entry issuer: %w", err)
	}
	issuer := strings.TrimSpace(bot.EntryIssuer)
	if issuer == "" {
		return "", errors.New("botlink: registered entry issuer is empty")
	}
	if presented := strings.TrimSpace(presentedIssuer); presented != "" && presented != issuer {
		return "", ErrEntryIssuerMismatch
	}
	return issuer, nil
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
		Issuer:         row.Issuer,
		UserID:         userID,
		ExternalUserID: row.ExternalUserID,
		RequestIP:      requestIP,
		RequestUA:      requestUA,
	})
}

func (s *Service) writeAudit(tx *gorm.DB, userID uint64, action string, in UpsertLinkInput) error {
	issuer := strings.TrimSpace(in.Issuer)
	details := map[string]string{
		"bot_id":           in.BotID,
		"issuer":           issuer,
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
