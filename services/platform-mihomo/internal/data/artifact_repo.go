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
			"binding_ref = ? AND artifact_type = ? AND scope_key = ?",
			artifact.BindingRef,
			artifact.ArtifactType,
			artifact.ScopeKey,
		).Take(&previous).Error
	}

	record := model.RuntimeArtifact{
		BindingRef:    artifact.BindingRef,
		AccountKey:    artifact.AccountKey,
		ArtifactType:  artifact.ArtifactType,
		ArtifactValue: artifact.ArtifactValue,
		ScopeKey:      artifact.ScopeKey,
		ExpiresAt:     artifact.ExpiresAt,
	}

	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "binding_ref"}, {Name: "artifact_type"}, {Name: "scope_key"}},
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
	if previous.AccountKey != "" && previous.AccountKey != artifact.AccountKey {
		if err := r.redis.Del(ctx, r.cacheKey(previous.AccountKey, artifact.ArtifactType, artifact.ScopeKey)).Err(); err != nil {
			return err
		}
	}
	if err := r.redis.Del(ctx, r.cacheKey(artifact.AccountKey, artifact.ArtifactType, artifact.ScopeKey)).Err(); err != nil {
		return err
	}

	ttl := time.Until(artifact.ExpiresAt)
	if ttl <= 0 {
		return r.redis.Del(ctx, r.cacheKeyByBinding(artifact.BindingRef, artifact.ArtifactType, artifact.ScopeKey)).Err()
	}

	return r.redis.Set(ctx, r.cacheKeyByBinding(artifact.BindingRef, artifact.ArtifactType, artifact.ScopeKey), payload, ttl).Err()
}

func (r *ArtifactRepo) GetByBindingRef(ctx context.Context, bindingRef string, artifactType, scopeKey string) (*biz.Artifact, error) {
	if r.redis != nil {
		payload, err := r.redis.Get(ctx, r.cacheKeyByBinding(bindingRef, artifactType, scopeKey)).Bytes()
		if err == nil {
			artifact := &biz.Artifact{}
			if err := json.Unmarshal(payload, artifact); err == nil {
				return artifact, nil
			}
		}
	}

	artifact, err := r.get(ctx, "binding_ref = ? AND artifact_type = ? AND scope_key = ? AND expires_at > ?", bindingRef, artifactType, scopeKey, time.Now())
	if err != nil || artifact == nil {
		return artifact, err
	}

	if r.redis != nil {
		payload, err := json.Marshal(artifact)
		if err == nil {
			ttl := time.Until(artifact.ExpiresAt)
			if ttl <= 0 {
				_ = r.redis.Del(ctx, r.cacheKeyByBinding(bindingRef, artifactType, scopeKey)).Err()
				return artifact, nil
			}
			_ = r.redis.Set(ctx, r.cacheKeyByBinding(bindingRef, artifactType, scopeKey), payload, ttl).Err()
		}
	}

	return artifact, nil
}

func (r *ArtifactRepo) Get(ctx context.Context, accountKey, artifactType, scopeKey string) (*biz.Artifact, error) {
	if r.redis != nil {
		payload, err := r.redis.Get(ctx, r.cacheKey(accountKey, artifactType, scopeKey)).Bytes()
		if err == nil {
			artifact := &biz.Artifact{}
			if err := json.Unmarshal(payload, artifact); err == nil {
				return artifact, nil
			}
		}
	}

	artifact, err := r.get(ctx,
		"account_key = ? AND artifact_type = ? AND scope_key = ? AND expires_at > ?",
		accountKey,
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
				_ = r.redis.Del(ctx, r.cacheKey(accountKey, artifactType, scopeKey)).Err()
				return artifact, nil
			}
			_ = r.redis.Set(ctx, r.cacheKey(accountKey, artifactType, scopeKey), payload, ttl).Err()
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
		BindingRef:    record.BindingRef,
		AccountKey:    record.AccountKey,
		ArtifactType:  record.ArtifactType,
		ArtifactValue: record.ArtifactValue,
		ScopeKey:      record.ScopeKey,
		ExpiresAt:     record.ExpiresAt,
	}
	return artifact, nil
}

func (r *ArtifactRepo) DeleteByAccountKey(ctx context.Context, accountKey string) error {
	return r.deleteByAccountKey(ctx, dbFromContext(ctx, r.db), accountKey)
}

func (r *ArtifactRepo) deleteByAccountKey(ctx context.Context, db *gorm.DB, accountKey string) error {
	records, err := r.recordsForPlatformCacheDeletion(ctx, db, accountKey)
	if err != nil {
		return err
	}

	if err := db.WithContext(ctx).Where("account_key = ?", accountKey).Delete(&model.RuntimeArtifact{}).Error; err != nil {
		return err
	}

	return r.deleteCacheKeys(ctx, r.cacheKeysForRecords(records))
}

func (r *ArtifactRepo) DeleteByBindingRef(ctx context.Context, bindingRef string) error {
	return r.deleteByBindingRef(ctx, dbFromContext(ctx, r.db), bindingRef)
}

func (r *ArtifactRepo) deleteByBindingRef(ctx context.Context, db *gorm.DB, bindingRef string) error {
	records, err := r.recordsForBindingCacheDeletion(ctx, db, bindingRef)
	if err != nil {
		return err
	}

	if err := db.WithContext(ctx).Where("binding_ref = ?", bindingRef).Delete(&model.RuntimeArtifact{}).Error; err != nil {
		return err
	}

	return r.deleteCacheKeys(ctx, r.cacheKeysForRecords(records))
}

func (r *ArtifactRepo) recordsForBindingCacheDeletion(ctx context.Context, db *gorm.DB, bindingRef string) ([]model.RuntimeArtifact, error) {
	if r.redis == nil {
		return nil, nil
	}
	var records []model.RuntimeArtifact
	err := db.WithContext(ctx).
		Select("binding_ref", "account_key", "artifact_type", "scope_key").
		Where("binding_ref = ?", bindingRef).
		Find(&records).Error
	return records, err
}

func (r *ArtifactRepo) recordsForPlatformCacheDeletion(ctx context.Context, db *gorm.DB, accountKey string) ([]model.RuntimeArtifact, error) {
	if r.redis == nil {
		return nil, nil
	}
	var records []model.RuntimeArtifact
	err := db.WithContext(ctx).
		Select("binding_ref", "account_key", "artifact_type", "scope_key").
		Where("account_key = ?", accountKey).
		Find(&records).Error
	return records, err

}

func (r *ArtifactRepo) cacheKeysForRecords(records []model.RuntimeArtifact) []string {
	keys := make([]string, 0, len(records)*2)
	for _, record := range records {
		keys = append(keys,
			r.cacheKey(record.AccountKey, record.ArtifactType, record.ScopeKey),
			r.cacheKeyByBinding(record.BindingRef, record.ArtifactType, record.ScopeKey),
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

func (r *ArtifactRepo) cacheKey(accountKey, artifactType, scopeKey string) string {
	return fmt.Sprintf("%sartifact:%s:%s:%s", r.prefix, accountKey, artifactType, scopeKey)
}

func (r *ArtifactRepo) cacheKeyByBinding(bindingRef string, artifactType, scopeKey string) string {
	return fmt.Sprintf("%sartifact:binding:%s:%s:%s", r.prefix, bindingRef, artifactType, scopeKey)
}

func (r *ArtifactRepo) bindingCachePattern(bindingRef string) string {
	return fmt.Sprintf("%sartifact:binding:%s:*", r.prefix, bindingRef)
}
