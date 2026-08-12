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
	return r.put(ctx, artifact)
}

func (r *ArtifactRepo) PutIfCredentialCurrent(ctx context.Context, artifact *biz.Artifact, expectedGeneration uint64) error {
	db := dbFromContext(ctx, r.db)
	var pending int64
	if err := db.Model(&model.RuntimeArtifact{}).Where("binding_ref = ? AND revocation_pending = TRUE", artifact.BindingRef).Count(&pending).Error; err != nil {
		return err
	}
	if pending > 0 {
		return biz.ErrArtifactRevocationPending
	}
	var current model.CredentialRecord
	err := db.Select("generation", "status", "expires_at").Where(
		"binding_ref = ? AND account_key = ? AND generation = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)",
		artifact.BindingRef,
		artifact.AccountKey,
		expectedGeneration,
		"active",
		time.Now(),
	).Take(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return biz.ErrArtifactCredentialStale
	}
	if err != nil {
		return err
	}
	return r.put(ctx, artifact)
}

func (r *ArtifactRepo) put(ctx context.Context, artifact *biz.Artifact) error {
	var previous model.RuntimeArtifact
	if r.redis != nil {
		_ = dbFromContext(ctx, r.db).WithContext(ctx).Where(
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

	if err := dbFromContext(ctx, r.db).WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "binding_ref"}, {Name: "artifact_type"}, {Name: "scope_key"}},
		UpdateAll: true,
	}).Create(&record).Error; err != nil {
		return err
	}

	if r.redis == nil || txFromContext(ctx) != nil {
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

func (r *ArtifactRepo) ListByBindingRef(ctx context.Context, bindingRef string) ([]*biz.Artifact, error) {
	var records []model.RuntimeArtifact
	if err := dbFromContext(ctx, r.db).Where("binding_ref = ?", bindingRef).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	artifacts := make([]*biz.Artifact, 0, len(records))
	for _, record := range records {
		artifacts = append(artifacts, artifactFromRecord(record))
	}
	return artifacts, nil
}

func (r *ArtifactRepo) HasRevocationPending(ctx context.Context, bindingRef string) (bool, error) {
	var count int64
	if err := dbFromContext(ctx, r.db).Model(&model.RuntimeArtifact{}).
		Where("binding_ref = ? AND revocation_pending = TRUE", bindingRef).
		Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := r.db.WithContext(ctx).Model(&model.ArtifactRevocationIntent{}).
		Where("binding_ref = ?", bindingRef).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ArtifactRepo) GetByBindingRef(ctx context.Context, bindingRef string, artifactType, scopeKey string) (*biz.Artifact, error) {
	artifact, err := r.get(ctx, "binding_ref = ? AND artifact_type = ? AND scope_key = ? AND revocation_pending = FALSE", bindingRef, artifactType, scopeKey)
	if err != nil || artifact == nil {
		return artifact, err
	}
	if !artifact.ExpiresAt.After(time.Now()) {
		return nil, r.deleteExpiredArtifact(ctx, artifact)
	}

	if r.redis != nil {
		ttl := time.Until(artifact.ExpiresAt)
		if ttl <= 0 {
			return nil, r.deleteExpiredArtifact(ctx, artifact)
		}
		payload, err := json.Marshal(artifact)
		if err == nil {
			_ = r.redis.Set(ctx, r.cacheKeyByBinding(bindingRef, artifactType, scopeKey), payload, ttl).Err()
		}
	}

	return artifact, nil
}

func (r *ArtifactRepo) Get(ctx context.Context, accountKey, artifactType, scopeKey string) (*biz.Artifact, error) {
	artifact, err := r.get(ctx,
		"account_key = ? AND artifact_type = ? AND scope_key = ? AND revocation_pending = FALSE",
		accountKey,
		artifactType,
		scopeKey)
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, nil
	}
	if !artifact.ExpiresAt.After(time.Now()) {
		return nil, r.deleteExpiredArtifact(ctx, artifact)
	}

	if r.redis != nil {
		ttl := time.Until(artifact.ExpiresAt)
		if ttl <= 0 {
			return nil, r.deleteExpiredArtifact(ctx, artifact)
		}
		payload, err := json.Marshal(artifact)
		if err == nil {
			_ = r.redis.Set(ctx, r.cacheKey(accountKey, artifactType, scopeKey), payload, ttl).Err()
		}
	}

	return artifact, nil
}

func (r *ArtifactRepo) deleteExpiredArtifact(ctx context.Context, artifact *biz.Artifact) error {
	db := dbFromContext(ctx, r.db)
	if err := db.WithContext(ctx).Where(
		"binding_ref = ? AND artifact_type = ? AND scope_key = ? AND expires_at <= ?",
		artifact.BindingRef,
		artifact.ArtifactType,
		artifact.ScopeKey,
		time.Now(),
	).Delete(&model.RuntimeArtifact{}).Error; err != nil {
		return err
	}
	return r.deleteCacheKeys(ctx, r.cacheKeysForArtifacts([]*biz.Artifact{artifact}))
}

func (r *ArtifactRepo) get(ctx context.Context, query any, args ...any) (*biz.Artifact, error) {
	var record model.RuntimeArtifact
	err := dbFromContext(ctx, r.db).WithContext(ctx).Where(query, args...).Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return artifactFromRecord(record), nil
}

func artifactFromRecord(record model.RuntimeArtifact) *biz.Artifact {
	return &biz.Artifact{
		BindingRef:    record.BindingRef,
		AccountKey:    record.AccountKey,
		ArtifactType:  record.ArtifactType,
		ArtifactValue: record.ArtifactValue,
		ScopeKey:      record.ScopeKey,
		ExpiresAt:     record.ExpiresAt,
	}
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

func (r *ArtifactRepo) DeleteByBindingRefImmediately(ctx context.Context, bindingRef string) error {
	return r.deleteByBindingRef(ctx, r.db, bindingRef)
}

func (r *ArtifactRepo) DeleteArtifactImmediately(ctx context.Context, bindingRef, artifactType, scopeKey, artifactValue string) error {
	var record model.RuntimeArtifact
	err := r.db.WithContext(ctx).Where(
		"binding_ref = ? AND artifact_type = ? AND scope_key = ? AND artifact_value = ?",
		bindingRef,
		artifactType,
		scopeKey,
		artifactValue,
	).Take(&record).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := r.db.WithContext(ctx).Where(
		"binding_ref = ? AND artifact_type = ? AND scope_key = ? AND artifact_value = ?",
		bindingRef,
		artifactType,
		scopeKey,
		artifactValue,
	).Delete(&model.RuntimeArtifact{}).Error; err != nil {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return r.deleteCacheKeys(ctx, r.cacheKeysForRecords([]model.RuntimeArtifact{record}))
}

func (r *ArtifactRepo) MarkRevocationPendingImmediately(ctx context.Context, bindingRef string) error {
	records, err := r.recordsForBindingCacheDeletion(ctx, r.db, bindingRef)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Model(&model.RuntimeArtifact{}).
		Where("binding_ref = ?", bindingRef).
		Update("revocation_pending", true).Error; err != nil {
		return err
	}
	return r.deleteCacheKeys(ctx, r.cacheKeysForRecords(records))
}

func (r *ArtifactRepo) DeleteExpired(ctx context.Context, expiredBefore time.Time) (int64, error) {
	db := dbFromContext(ctx, r.db)
	var records []model.RuntimeArtifact
	if r.redis != nil {
		if err := db.WithContext(ctx).
			Select("binding_ref", "account_key", "artifact_type", "scope_key").
			Where("expires_at <= ? AND revocation_pending = FALSE", expiredBefore).
			Find(&records).Error; err != nil {
			return 0, err
		}
	}
	deleted := db.WithContext(ctx).Where("expires_at <= ? AND revocation_pending = FALSE", expiredBefore).Delete(&model.RuntimeArtifact{})
	if deleted.Error != nil {
		return 0, deleted.Error
	}
	if err := r.deleteCacheKeys(ctx, r.cacheKeysForRecords(records)); err != nil {
		return 0, err
	}
	return deleted.RowsAffected, nil
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

func (r *ArtifactRepo) cacheKeysForArtifacts(artifacts []*biz.Artifact) []string {
	keys := make([]string, 0, len(artifacts)*2)
	for _, artifact := range artifacts {
		keys = append(keys,
			r.cacheKey(artifact.AccountKey, artifact.ArtifactType, artifact.ScopeKey),
			r.cacheKeyByBinding(artifact.BindingRef, artifact.ArtifactType, artifact.ScopeKey),
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
