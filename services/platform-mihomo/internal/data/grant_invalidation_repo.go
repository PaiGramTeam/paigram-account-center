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

func (r *GrantInvalidationRepo) Upsert(ctx context.Context, bindingRef string, consumer string, minimumVersion uint64) error {
	now := time.Now().UTC()
	row := model.ConsumerGrantInvalidation{
		BindingRef:          bindingRef,
		Consumer:            consumer,
		MinimumGrantVersion: minimumVersion,
		InvalidatedAt:       now,
	}

	return dbFromContext(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "binding_ref"}, {Name: "consumer"}},
		DoUpdates: clause.Assignments(map[string]any{
			"minimum_grant_version": gorm.Expr("CASE WHEN consumer_grant_invalidations.minimum_grant_version > excluded.minimum_grant_version THEN consumer_grant_invalidations.minimum_grant_version ELSE excluded.minimum_grant_version END"),
			"invalidated_at":        now,
			"updated_at":            now,
		}),
	}).Create(&row).Error
}

func (r *GrantInvalidationRepo) MinimumVersion(ctx context.Context, bindingRef string, consumer string) (uint64, error) {
	var row model.ConsumerGrantInvalidation
	err := r.db.WithContext(ctx).Where("binding_ref = ? AND consumer = ?", bindingRef, consumer).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return row.MinimumGrantVersion, nil
}
