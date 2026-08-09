package data

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"platform-mihomo-service/internal/data/model"
)

type GrantInvalidationRepo struct {
	db *gorm.DB
}

func NewGrantInvalidationRepo(db *gorm.DB) *GrantInvalidationRepo {
	return &GrantInvalidationRepo{db: db}
}

func (r *GrantInvalidationRepo) Upsert(ctx context.Context, bindingID uint64, consumer string, minimumVersion uint64) error {
	now := time.Now().UTC()
	row := model.ConsumerGrantInvalidation{
		BindingID:           bindingID,
		Consumer:            consumer,
		MinimumGrantVersion: minimumVersion,
		InvalidatedAt:       now,
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "binding_id"}, {Name: "consumer"}},
		DoUpdates: clause.Assignments(map[string]any{
			"minimum_grant_version": gorm.Expr("CASE WHEN minimum_grant_version > ? THEN minimum_grant_version ELSE ? END", minimumVersion, minimumVersion),
			"invalidated_at":        now,
			"updated_at":            now,
		}),
	}).Create(&row).Error
}

func (r *GrantInvalidationRepo) MinimumVersion(ctx context.Context, bindingID uint64, consumer string) (uint64, error) {
	var row model.ConsumerGrantInvalidation
	err := r.db.WithContext(ctx).Where("binding_id = ? AND consumer = ?", bindingID, consumer).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return row.MinimumGrantVersion, nil
}
