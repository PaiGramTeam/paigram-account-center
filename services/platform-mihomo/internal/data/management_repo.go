package data

import (
	"context"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"platform-mihomo-service/internal/data/model"
)

type ManagementRepo struct {
	db     *gorm.DB
	redis  *redis.Client
	prefix string
}

func NewManagementRepo(db *gorm.DB, redisClient *redis.Client, prefix string) *ManagementRepo {
	return &ManagementRepo{db: db, redis: redisClient, prefix: prefix}
}

func (r *ManagementRepo) DeleteCredentialGraph(ctx context.Context, platformAccountID string) error {
	artifactRepo := NewArtifactRepo(r.db, r.redis, r.prefix)
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bindingIDs, err := r.bindingIDsForPlatformAccountID(ctx, tx, platformAccountID)
		if err != nil {
			return err
		}
		if len(bindingIDs) == 0 {
			if err := artifactRepo.deleteByPlatformAccountID(ctx, tx, platformAccountID); err != nil {
				return err
			}
		} else {
			for _, bindingID := range bindingIDs {
				if err := artifactRepo.deleteByBindingID(ctx, tx, bindingID); err != nil {
					return err
				}
			}
		}
		if err := tx.Where("platform_account_id = ?", platformAccountID).Delete(&model.AccountProfile{}).Error; err != nil {
			return err
		}
		if err := tx.Where("platform_account_id = ?", platformAccountID).Delete(&model.DeviceRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("platform_account_id = ?", platformAccountID).Delete(&model.CredentialRecord{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (r *ManagementRepo) bindingIDsForPlatformAccountID(ctx context.Context, tx *gorm.DB, platformAccountID string) ([]uint64, error) {
	var rows []struct {
		BindingID uint64
	}
	err := tx.WithContext(ctx).Raw(`
		SELECT DISTINCT binding_id FROM credential_records WHERE platform_account_id = ?
		UNION
		SELECT DISTINCT binding_id FROM runtime_artifacts WHERE platform_account_id = ?
		UNION
		SELECT DISTINCT binding_id FROM account_profiles WHERE platform_account_id = ?
		UNION
		SELECT DISTINCT binding_id FROM device_records WHERE platform_account_id = ?
	`, platformAccountID, platformAccountID, platformAccountID, platformAccountID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	bindingIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if row.BindingID != 0 {
			bindingIDs = append(bindingIDs, row.BindingID)
		}
	}
	return bindingIDs, nil
}

func (r *ManagementRepo) DeleteCredentialGraphByBindingID(ctx context.Context, bindingID uint64) error {
	artifactRepo := NewArtifactRepo(r.db, r.redis, r.prefix)
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := artifactRepo.deleteByBindingID(ctx, tx, bindingID); err != nil {
			return err
		}
		if err := tx.Where("binding_id = ?", bindingID).Delete(&model.AccountProfile{}).Error; err != nil {
			return err
		}
		if err := tx.Where("binding_id = ?", bindingID).Delete(&model.DeviceRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("binding_id = ?", bindingID).Delete(&model.CredentialRecord{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}
