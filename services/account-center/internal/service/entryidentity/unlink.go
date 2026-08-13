package entryidentity

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"paigram/internal/model"
	"paigram/internal/service/botlink"
)

func (s *Service) Unlink(ctx context.Context, userID uint64, botID, operationID, requestIP, requestUA string) (*UnlinkResult, error) {
	operationID = strings.TrimSpace(operationID)
	if s == nil || s.db == nil || s.grants == nil || userID == 0 || strings.TrimSpace(botID) == "" || !validOperationID(operationID) {
		return nil, ErrInvalidInput
	}
	operation := model.EntryIdentityUnlinkOperation{}
	grantIDs := make([]uint64, 0)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}
		found, err := loadUnlinkOperation(tx, operationID, &operation)
		if err != nil {
			return err
		}
		if found {
			if operation.UserID != userID || operation.BotID != botID {
				return botlink.ErrBotIdentityNotFound
			}
			return loadUnlinkTargetIDs(tx, operationID, &grantIDs)
		}
		return s.admitUnlink(ctx, tx, userID, botID, operationID, requestIP, requestUA, &operation, &grantIDs)
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	if operation.State == model.EntryIdentityUnlinkComplete {
		return unlinkResult(operation), nil
	}
	for _, grantID := range grantIDs {
		if err := s.grants.ReconcileGrantInvalidation(ctx, grantID); err != nil {
			s.logger.Warn("entry identity revocation propagation pending", zap.Uint64("grant_id", grantID), zap.Error(err))
		}
	}
	return s.refreshUnlinkState(ctx, userID, botID, operationID)
}

func (s *Service) UnlinkStatus(ctx context.Context, userID uint64, botID, operationID string) (*UnlinkResult, error) {
	operationID = strings.TrimSpace(operationID)
	if s == nil || s.db == nil || userID == 0 || strings.TrimSpace(botID) == "" || !validOperationID(operationID) {
		return nil, ErrInvalidInput
	}
	return s.refreshUnlinkState(ctx, userID, botID, operationID)
}

func (s *Service) admitUnlink(
	ctx context.Context,
	tx *gorm.DB,
	userID uint64,
	botID string,
	operationID string,
	requestIP string,
	requestUA string,
	operation *model.EntryIdentityUnlinkOperation,
	grantIDs *[]uint64,
) error {
	var identity model.BotIdentity
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND bot_id = ?", userID, botID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return botlink.ErrBotIdentityNotFound
		}
		return err
	}
	var maximum uint64
	if err := tx.Unscoped().Model(&model.BotIdentity{}).Where("user_id = ?", userID).
		Select("COALESCE(MAX(entry_epoch), 1)").Scan(&maximum).Error; err != nil {
		return err
	}
	minimumEntryEpoch := maximum + 1
	if err := tx.Model(&model.BotIdentity{}).Where("user_id = ?", userID).
		Update("entry_epoch", minimumEntryEpoch).Error; err != nil {
		return err
	}
	if err := collectIdentityGrantIDs(tx, userID, identity.BotID, grantIDs); err != nil {
		return err
	}
	state := model.EntryIdentityUnlinkPending
	completedAt := sql.NullTime{}
	if len(*grantIDs) == 0 {
		state = model.EntryIdentityUnlinkComplete
		completedAt = sql.NullTime{Time: s.now().UTC(), Valid: true}
	}
	*operation = model.EntryIdentityUnlinkOperation{
		OperationID: operationID, UserID: userID, BotID: botID, EntryIdentityRef: identity.EntryIdentityRef,
		MinimumEntryEpoch: minimumEntryEpoch, State: state, CompletedAt: completedAt,
	}
	if err := tx.Create(operation).Error; err != nil {
		return err
	}
	for _, grantID := range *grantIDs {
		if err := tx.Create(&model.EntryIdentityUnlinkTarget{OperationID: operationID, GrantID: grantID}).Error; err != nil {
			return err
		}
	}
	if len(*grantIDs) > 0 {
		pendingEpoch := gorm.Expr("CASE WHEN pending_entry_epoch > ? THEN pending_entry_epoch ELSE ? END", minimumEntryEpoch, minimumEntryEpoch)
		if err := tx.Model(&model.ConsumerGrant{}).Where("id IN ?", *grantIDs).
			Updates(map[string]any{"pending_entry_epoch": pendingEpoch, "last_invalidated_at": nil}).Error; err != nil {
			return err
		}
	}
	return s.linker.UnlinkTx(ctx, tx, userID, botID, requestIP, requestUA)
}

func (s *Service) refreshUnlinkState(ctx context.Context, userID uint64, botID, operationID string) (*UnlinkResult, error) {
	operation := model.EntryIdentityUnlinkOperation{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id = ?", operationID).Take(&operation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return botlink.ErrBotIdentityNotFound
			}
			return err
		}
		if operation.UserID != userID || operation.BotID != botID {
			return botlink.ErrBotIdentityNotFound
		}
		if operation.State == model.EntryIdentityUnlinkComplete {
			return nil
		}
		var pending int64
		if err := tx.Model(&model.EntryIdentityUnlinkTarget{}).
			Where("operation_id = ? AND confirmed_at IS NULL", operationID).
			Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return nil
		}
		now := s.now().UTC()
		if err := tx.Model(&operation).Updates(map[string]any{
			"state": model.EntryIdentityUnlinkComplete, "completed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		operation.State = model.EntryIdentityUnlinkComplete
		operation.CompletedAt = sql.NullTime{Time: now, Valid: true}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return unlinkResult(operation), nil
}

func loadUnlinkOperation(tx *gorm.DB, operationID string, operation *model.EntryIdentityUnlinkOperation) (bool, error) {
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id = ?", operationID).Take(operation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}

func loadUnlinkTargetIDs(tx *gorm.DB, operationID string, grantIDs *[]uint64) error {
	return tx.Model(&model.EntryIdentityUnlinkTarget{}).
		Where("operation_id = ? AND confirmed_at IS NULL", operationID).
		Pluck("grant_id", grantIDs).Error
}

func collectIdentityGrantIDs(tx *gorm.DB, userID uint64, botID string, grantIDs *[]uint64) error {
	var consumers []string
	if err := tx.Unscoped().Model(&model.ServiceCredential{}).Where("bot_id = ?", botID).Pluck("client_id", &consumers).Error; err != nil {
		return err
	}
	if len(consumers) == 0 {
		return nil
	}
	return tx.Model(&model.ConsumerGrant{}).
		Joins("JOIN platform_account_bindings ON platform_account_bindings.id = consumer_grants.binding_id").
		Where("platform_account_bindings.owner_user_id = ? AND consumer_grants.consumer IN ?", userID, consumers).
		Pluck("consumer_grants.id", grantIDs).Error
}

func unlinkResult(operation model.EntryIdentityUnlinkOperation) *UnlinkResult {
	pending := operation.State != model.EntryIdentityUnlinkComplete
	return &UnlinkResult{
		OperationID: operation.OperationID, MinimumEntryEpoch: operation.MinimumEntryEpoch,
		PropagationPending: pending, State: string(operation.State),
	}
}

func validOperationID(operationID string) bool {
	parsed, err := uuid.Parse(operationID)
	return err == nil && parsed != uuid.Nil && parsed.String() == operationID
}
