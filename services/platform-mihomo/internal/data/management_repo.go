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

func (r *ManagementRepo) DeleteCredentialGraph(ctx context.Context, accountKey string) error {
	artifactRepo := NewArtifactRepo(r.db, r.redis, r.prefix)
	deleteGraph := func(tx *gorm.DB) error {
		bindingRefs, err := r.bindingRefsForAccountKey(ctx, tx, accountKey)
		if err != nil {
			return err
		}
		if len(bindingRefs) == 0 {
			if err := artifactRepo.deleteByAccountKey(ctx, tx, accountKey); err != nil {
				return err
			}
		} else {
			for _, bindingRef := range bindingRefs {
				if err := artifactRepo.deleteByBindingRef(ctx, tx, bindingRef); err != nil {
					return err
				}
			}
		}
		if err := tx.Where("account_key = ?", accountKey).Delete(&model.AccountProfile{}).Error; err != nil {
			return err
		}
		if err := tx.Where("account_key = ?", accountKey).Delete(&model.DeviceRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("account_key = ?", accountKey).Delete(&model.CredentialRecord{}).Error; err != nil {
			return err
		}
		return nil
	}
	if err := r.runTransaction(ctx, deleteGraph); err != nil {
		return err
	}

	return nil
}

func (r *ManagementRepo) bindingRefsForAccountKey(ctx context.Context, tx *gorm.DB, accountKey string) ([]string, error) {
	var rows []struct {
		BindingRef string
	}
	err := tx.WithContext(ctx).Raw(`
		SELECT DISTINCT binding_ref FROM credential_records WHERE account_key = ?
		UNION
		SELECT DISTINCT binding_ref FROM runtime_artifacts WHERE account_key = ?
		UNION
		SELECT DISTINCT binding_ref FROM account_profiles WHERE account_key = ?
		UNION
		SELECT DISTINCT binding_ref FROM device_records WHERE account_key = ?
	`, accountKey, accountKey, accountKey, accountKey).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	bindingRefs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.BindingRef != "" {
			bindingRefs = append(bindingRefs, row.BindingRef)
		}
	}
	return bindingRefs, nil
}

func (r *ManagementRepo) DeleteCredentialGraphByBindingRef(ctx context.Context, bindingRef string) error {
	artifactRepo := NewArtifactRepo(r.db, r.redis, r.prefix)
	deleteGraph := func(tx *gorm.DB) error {
		if err := artifactRepo.deleteByBindingRef(ctx, tx, bindingRef); err != nil {
			return err
		}
		if err := tx.Where("binding_ref = ?", bindingRef).Delete(&model.AccountProfile{}).Error; err != nil {
			return err
		}
		if err := tx.Where("binding_ref = ?", bindingRef).Delete(&model.DeviceRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("binding_ref = ?", bindingRef).Delete(&model.CredentialRecord{}).Error; err != nil {
			return err
		}
		return nil
	}
	if err := r.runTransaction(ctx, deleteGraph); err != nil {
		return err
	}

	return nil
}

func (r *ManagementRepo) runTransaction(ctx context.Context, fn func(*gorm.DB) error) error {
	if tx := txFromContext(ctx); tx != nil {
		return fn(tx.WithContext(ctx))
	}
	return r.db.WithContext(ctx).Transaction(fn)
}
