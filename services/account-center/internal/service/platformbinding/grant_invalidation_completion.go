package platformbinding

import (
	"database/sql"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"paigram/internal/model"
)

func (s *GrantService) completeGrantInvalidation(grant *model.ConsumerGrant, minimumVersion, minimumEntryEpoch uint64, invalidatedAt time.Time) error {
	if grant == nil || grant.ID == 0 {
		return nil
	}
	var current model.ConsumerGrant
	err := s.db.Transaction(func(tx *gorm.DB) error {
		update := tx.Model(&model.ConsumerGrant{}).
			Where("id = ? AND ticket_version = ? AND pending_entry_epoch = ?", grant.ID, minimumVersion, minimumEntryEpoch).
			Updates(map[string]any{"last_invalidated_at": invalidatedAt, "pending_entry_epoch": 0})
		if update.Error != nil {
			return update.Error
		}
		if err := tx.Preload("Actions").Where("id = ?", grant.ID).First(&current).Error; err != nil {
			return err
		}
		if update.RowsAffected == 0 &&
			(current.TicketVersion != minimumVersion || current.PendingEntryEpoch != 0 || !current.LastInvalidatedAt.Valid) {
			return ErrGrantPropagationPending
		}
		if minimumEntryEpoch == 0 {
			return nil
		}
		var operationIDs []string
		if err := tx.Model(&model.EntryIdentityUnlinkOperation{}).
			Select("entry_identity_unlink_operations.operation_id").
			Joins("JOIN entry_identity_unlink_targets ON entry_identity_unlink_targets.operation_id = entry_identity_unlink_operations.operation_id").
			Where("entry_identity_unlink_targets.grant_id = ? AND entry_identity_unlink_targets.confirmed_at IS NULL", grant.ID).
			Where("entry_identity_unlink_operations.minimum_entry_epoch <= ?", minimumEntryEpoch).
			Order("entry_identity_unlink_operations.operation_id ASC").
			Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: clause.CurrentTable}}).
			Pluck("entry_identity_unlink_operations.operation_id", &operationIDs).Error; err != nil {
			return err
		}
		if len(operationIDs) == 0 {
			return nil
		}
		confirmed := sql.NullTime{Time: invalidatedAt, Valid: true}
		if err := tx.Model(&model.EntryIdentityUnlinkTarget{}).
			Where("grant_id = ? AND confirmed_at IS NULL AND operation_id IN ?", grant.ID, operationIDs).
			Update("confirmed_at", confirmed).Error; err != nil {
			return err
		}
		return completeConfirmedUnlinkOperations(tx, operationIDs, invalidatedAt)
	})
	if err != nil {
		return err
	}
	*grant = current
	return nil
}

func completeConfirmedUnlinkOperations(tx *gorm.DB, operationIDs []string, completedAt time.Time) error {
	return tx.Model(&model.EntryIdentityUnlinkOperation{}).
		Where("state = ? AND operation_id IN ?", model.EntryIdentityUnlinkPending, operationIDs).
		Where(`NOT EXISTS (
			SELECT 1 FROM entry_identity_unlink_targets
			WHERE entry_identity_unlink_targets.operation_id = entry_identity_unlink_operations.operation_id
			AND entry_identity_unlink_targets.confirmed_at IS NULL
		)`).
		Updates(map[string]any{
			"state": model.EntryIdentityUnlinkComplete, "completed_at": completedAt, "updated_at": completedAt,
		}).Error
}
