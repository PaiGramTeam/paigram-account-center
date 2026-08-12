package platformbinding

import (
	"time"

	"gorm.io/gorm"

	"paigram/internal/model"
)

func (s *GrantService) completeGrantInvalidation(grant *model.ConsumerGrant, minimumVersion uint64, invalidatedAt time.Time) error {
	if grant == nil || grant.ID == 0 {
		return nil
	}
	update := s.db.Model(&model.ConsumerGrant{}).
		Where("id = ? AND status = ? AND ticket_version = ?", grant.ID, model.ConsumerGrantStatusRevoked, minimumVersion).
		Update("last_invalidated_at", invalidatedAt)
	if update.Error != nil {
		return update.Error
	}

	var current model.ConsumerGrant
	if err := s.db.Preload("Actions").Where("id = ?", grant.ID).First(&current).Error; err != nil {
		return err
	}
	if update.RowsAffected == 0 && current.TicketVersion < minimumVersion {
		return gorm.ErrRecordNotFound
	}
	*grant = current
	return nil
}
