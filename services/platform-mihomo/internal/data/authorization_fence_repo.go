package data

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data/model"
)

type AuthorizationFenceRepo struct {
	db *gorm.DB
}

func NewAuthorizationFenceRepo(db *gorm.DB) *AuthorizationFenceRepo {
	return &AuthorizationFenceRepo{db: db}
}

func (r *AuthorizationFenceRepo) Upsert(ctx context.Context, fence biz.AuthorizationFence) error {
	record := model.AuthorizationFence{
		BindingRef:           fence.BindingRef,
		ConsumerPrincipal:    fence.ConsumerPrincipal,
		MinimumGrantVersion:  fence.MinimumGrantVersion,
		MinimumOwnerEpoch:    fence.MinimumOwnerEpoch,
		MinimumConsumerEpoch: fence.MinimumConsumerEpoch,
		MinimumEntryEpoch:    fence.MinimumEntryEpoch,
	}
	return dbFromContext(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "binding_ref"}, {Name: "consumer_principal"}},
		DoUpdates: clause.Assignments(map[string]any{
			"minimum_grant_version":  gorm.Expr("GREATEST(authorization_fences.minimum_grant_version, excluded.minimum_grant_version)"),
			"minimum_owner_epoch":    gorm.Expr("GREATEST(authorization_fences.minimum_owner_epoch, excluded.minimum_owner_epoch)"),
			"minimum_consumer_epoch": gorm.Expr("GREATEST(authorization_fences.minimum_consumer_epoch, excluded.minimum_consumer_epoch)"),
			"minimum_entry_epoch":    gorm.Expr("GREATEST(authorization_fences.minimum_entry_epoch, excluded.minimum_entry_epoch)"),
			"updated_at":             gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(&record).Error
}
