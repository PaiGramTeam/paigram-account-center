package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data/model"
)

type ArtifactRepo struct {
	db     *gorm.DB
	redis  *redis.Client
	prefix string
}

func NewArtifactRepo(db *gorm.DB, redisClient *redis.Client, prefix string) *ArtifactRepo {
	return &ArtifactRepo{db: db, redis: redisClient, prefix: prefix}
}

func (r *ArtifactRepo) Put(ctx context.Context, artifact *biz.Artifact) error {
	var previous model.RuntimeArtifact
	if r.redis != nil {
		_ = r.db.WithContext(ctx).Where(
			"binding_id = ? AND artifact_type = ? AND scope_key = ?",
			artifact.BindingID,
			artifact.ArtifactType,
			artifact.ScopeKey,
		).Take(&previous).Error
	}

	record := model.RuntimeArtifact{
		BindingID:         artifact.BindingID,
		PlatformAccountID: artifact.PlatformAccountID,
		ArtifactType:      artifact.ArtifactType,
		ArtifactValue:     artifact.ArtifactValue,
		ScopeKey:          artifact.ScopeKey,
		ExpiresAt:         artifact.ExpiresAt,
	}

	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "binding_id"}, {Name: "artifact_type"}, {Name: "scope_key"}},
		UpdateAll: true,
	}).Create(&record).Error; err != nil {
		return err
	}

	if r.redis == nil {
		return nil
	}

	payload, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	if previous.PlatformAccountID != "" && previous.PlatformAccountID != artifact.PlatformAccountID {
		if err := r.redis.Del(ctx, r.cacheKey(previous.PlatformAccountID, artifact.ArtifactType, artifact.ScopeKey)).Err(); err != nil {
			return err
		}
	}
	if err := r.redis.Del(ctx, r.cacheKey(artifact.PlatformAccountID, artifact.ArtifactType, artifact.ScopeKey)).Err(); err != nil {
		return err
	}

	ttl := time.Until(artifact.ExpiresAt)
	if ttl <= 0 {
		return r.redis.Del(ctx, r.cacheKeyByBinding(artifact.BindingID, artifact.ArtifactType, artifact.ScopeKey)).Err()
	}

	return r.redis.Set(ctx, r.cacheKeyByBinding(artifact.BindingID, artifact.ArtifactType, artifact.ScopeKey), payload, ttl).Err()
}

func (r *ArtifactRepo) GetByBindingID(ctx context.Context, bindingID uint64, artifactType, scopeKey string) (*biz.Artifact, error) {
	if r.redis != nil {
		payload, err := r.redis.Get(ctx, r.cacheKeyByBinding(bindingID, artifactType, scopeKey)).Bytes()
		if err == nil {
			artifact := &biz.Artifact{}
			if err := json.Unmarshal(payload, artifact); err == nil {
				return artifact, nil
			}
		}
	}

	artifact, err := r.get(ctx, "binding_id = ? AND artifact_type = ? AND scope_key = ? AND expires_at > ?", bindingID, artifactType, scopeKey, time.Now())
	if err != nil || artifact == nil {
		return artifact, err
	}

	if r.redis != nil {
		payload, err := json.Marshal(artifact)
		if err == nil {
			ttl := time.Until(artifact.ExpiresAt)
			if ttl <= 0 {
				_ = r.redis.Del(ctx, r.cacheKeyByBinding(bindingID, artifactType, scopeKey)).Err()
				return artifact, nil
			}
			_ = r.redis.Set(ctx, r.cacheKeyByBinding(bindingID, artifactType, scopeKey), payload, ttl).Err()
		}
	}

	return artifact, nil
}

func (r *ArtifactRepo) Get(ctx context.Context, platformAccountID, artifactType, scopeKey string) (*biz.Artifact, error) {
	if r.redis != nil {
		payload, err := r.redis.Get(ctx, r.cacheKey(platformAccountID, artifactType, scopeKey)).Bytes()
		if err == nil {
			artifact := &biz.Artifact{}
			if err := json.Unmarshal(payload, artifact); err == nil {
				return artifact, nil
			}
		}
	}

	artifact, err := r.get(ctx,
		"platform_account_id = ? AND artifact_type = ? AND scope_key = ? AND expires_at > ?",
		platformAccountID,
		artifactType,
		scopeKey,
		time.Now())
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, nil
	}

	if r.redis != nil {
		payload, err := json.Marshal(artifact)
		if err == nil {
			ttl := time.Until(artifact.ExpiresAt)
			if ttl <= 0 {
				_ = r.redis.Del(ctx, r.cacheKey(platformAccountID, artifactType, scopeKey)).Err()
				return artifact, nil
			}
			_ = r.redis.Set(ctx, r.cacheKey(platformAccountID, artifactType, scopeKey), payload, ttl).Err()
		}
	}

	return artifact, nil
}

func (r *ArtifactRepo) get(ctx context.Context, query any, args ...any) (*biz.Artifact, error) {
	var record model.RuntimeArtifact
	err := r.db.WithContext(ctx).Where(query, args...).Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	artifact := &biz.Artifact{
		BindingID:         record.BindingID,
		PlatformAccountID: record.PlatformAccountID,
		ArtifactType:      record.ArtifactType,
		ArtifactValue:     record.ArtifactValue,
		ScopeKey:          record.ScopeKey,
		ExpiresAt:         record.ExpiresAt,
	}
	return artifact, nil
}

func (r *ArtifactRepo) DeleteByPlatformAccountID(ctx context.Context, platformAccountID string) error {
	return r.deleteByPlatformAccountID(ctx, dbFromContext(ctx, r.db), platformAccountID)
}

func (r *ArtifactRepo) deleteByPlatformAccountID(ctx context.Context, db *gorm.DB, platformAccountID string) error {
	records, err := r.recordsForPlatformCacheDeletion(ctx, db, platformAccountID)
	if err != nil {
		return err
	}

	if err := db.WithContext(ctx).Where("platform_account_id = ?", platformAccountID).Delete(&model.RuntimeArtifact{}).Error; err != nil {
		return err
	}

	return r.deleteCacheKeys(ctx, r.cacheKeysForRecords(records))
}

func (r *ArtifactRepo) DeleteByBindingID(ctx context.Context, bindingID uint64) error {
	return r.deleteByBindingID(ctx, dbFromContext(ctx, r.db), bindingID)
}

func (r *ArtifactRepo) deleteByBindingID(ctx context.Context, db *gorm.DB, bindingID uint64) error {
	records, err := r.recordsForBindingCacheDeletion(ctx, db, bindingID)
	if err != nil {
		return err
	}

	if err := db.WithContext(ctx).Where("binding_id = ?", bindingID).Delete(&model.RuntimeArtifact{}).Error; err != nil {
		return err
	}

	return r.deleteCacheKeys(ctx, r.cacheKeysForRecords(records))
}

func (r *ArtifactRepo) recordsForBindingCacheDeletion(ctx context.Context, db *gorm.DB, bindingID uint64) ([]model.RuntimeArtifact, error) {
	if r.redis == nil {
		return nil, nil
	}
	var records []model.RuntimeArtifact
	err := db.WithContext(ctx).
		Select("binding_id", "platform_account_id", "artifact_type", "scope_key").
		Where("binding_id = ?", bindingID).
		Find(&records).Error
	return records, err
}

func (r *ArtifactRepo) recordsForPlatformCacheDeletion(ctx context.Context, db *gorm.DB, platformAccountID string) ([]model.RuntimeArtifact, error) {
	if r.redis == nil {
		return nil, nil
	}
	var records []model.RuntimeArtifact
	err := db.WithContext(ctx).
		Select("binding_id", "platform_account_id", "artifact_type", "scope_key").
		Where("platform_account_id = ?", platformAccountID).
		Find(&records).Error
	return records, err

}

func (r *ArtifactRepo) cacheKeysForRecords(records []model.RuntimeArtifact) []string {
	keys := make([]string, 0, len(records)*2)
	for _, record := range records {
		keys = append(keys,
			r.cacheKey(record.PlatformAccountID, record.ArtifactType, record.ScopeKey),
			r.cacheKeyByBinding(record.BindingID, record.ArtifactType, record.ScopeKey),
		)
	}
	return keys

}

func (r *ArtifactRepo) deleteCacheKeys(ctx context.Context, keys []string) error {
	if r.redis == nil || len(keys) == 0 {
		return nil
	}
	return r.redis.Del(ctx, keys...).Err()
}

func (r *ArtifactRepo) cacheKey(platformAccountID, artifactType, scopeKey string) string {
	return fmt.Sprintf("%sartifact:%s:%s:%s", r.prefix, platformAccountID, artifactType, scopeKey)
}

func (r *ArtifactRepo) cacheKeyByBinding(bindingID uint64, artifactType, scopeKey string) string {
	return fmt.Sprintf("%sartifact:binding:%d:%s:%s", r.prefix, bindingID, artifactType, scopeKey)
}

func (r *ArtifactRepo) bindingCachePattern(bindingID uint64) string {
	return fmt.Sprintf("%sartifact:binding:%d:*", r.prefix, bindingID)
}
