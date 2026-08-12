package data

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data/model"
)

type CredentialRepo struct {
	db *gorm.DB
}

func NewCredentialRepo(db *gorm.DB) *CredentialRepo {
	return &CredentialRepo{db: db}
}

func (r *CredentialRepo) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if txFromContext(ctx) != nil {
		return fn(ctx)
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(withTx(ctx, tx))
	})
}

func (r *CredentialRepo) Save(ctx context.Context, credential *biz.Credential) error {
	record := credentialRecord(credential)

	return dbFromContext(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "binding_ref"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"account_key", "platform", "account_id", "region", "credential_blob", "credential_version", "status", "last_validated_at", "last_refreshed_at", "expires_at", "profile_snapshot_complete", "profile_revision", "profile_observed_revision", "updated_at",
		}),
	}).Create(&record).Error
}

func (r *CredentialRepo) SetProfileSnapshotState(ctx context.Context, bindingRef string, complete bool, revision, observedRevision uint64) error {
	write := dbFromContext(ctx, r.db).Model(&model.CredentialRecord{}).
		Where("binding_ref = ?", bindingRef).
		Updates(map[string]any{
			"profile_snapshot_complete": complete,
			"profile_revision":          revision,
			"profile_observed_revision": observedRevision,
		})
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return biz.ErrCredentialGenerationConflict
	}
	return nil
}

func (r *CredentialRepo) Create(ctx context.Context, credential *biz.Credential) error {
	record := credentialRecord(credential)
	if err := dbFromContext(ctx, r.db).Create(&record).Error; err != nil {
		return mapCredentialDuplicateError(err)
	}
	return nil
}

func (r *CredentialRepo) AdvanceGeneration(ctx context.Context, bindingRef, accountKey string, expected, target uint64) (*biz.Credential, error) {
	write := dbFromContext(ctx, r.db).Model(&model.CredentialRecord{}).
		Where("binding_ref = ? AND account_key = ? AND generation = ?", bindingRef, accountKey, expected).
		Update("generation", target)
	if write.Error != nil {
		return nil, write.Error
	}
	if write.RowsAffected != 1 {
		return nil, biz.ErrCredentialGenerationConflict
	}
	return r.GetByBindingRef(ctx, bindingRef)
}

func credentialRecord(credential *biz.Credential) model.CredentialRecord {
	return model.CredentialRecord{
		BindingRef:        credential.BindingRef,
		AccountKey:        credential.AccountKey,
		Generation:        credential.Generation,
		Platform:          credential.Platform,
		AccountID:         credential.AccountID,
		Region:            credential.Region,
		CredentialBlob:    credential.CredentialBlob,
		CredentialVersion: credential.CredentialVersion,
		Status:            credential.Status,
		LastValidatedAt:   credential.LastValidatedAt,
		LastRefreshedAt:   credential.LastRefreshedAt,
		ExpiresAt:         credential.ExpiresAt,
		ProfileSnapshotComplete: credential.ProfileSnapshotComplete,
		ProfileRevision:         credential.ProfileRevision,
		ProfileObservedRevision: credential.ProfileObservedRevision,
	}
}

func mapCredentialDuplicateError(err error) error {
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) && postgresErr.Code == "23505" {
		return biz.ErrCredentialAlreadyBound
	}
	if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
		return biz.ErrCredentialAlreadyBound
	}
	return err
}

func (r *CredentialRepo) GetByBindingRef(ctx context.Context, bindingRef string) (*biz.Credential, error) {
	var record model.CredentialRecord
	if err := dbFromContext(ctx, r.db).Where("binding_ref = ?", bindingRef).Order("id asc").Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return credentialFromRecord(record), nil
}

func (r *CredentialRepo) GetByAccountKey(ctx context.Context, accountKey string) (*biz.Credential, error) {
	var record model.CredentialRecord
	if err := dbFromContext(ctx, r.db).Where("account_key = ?", accountKey).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return credentialFromRecord(record), nil
}

func (r *CredentialRepo) DeleteByAccountKey(ctx context.Context, accountKey string) error {
	return dbFromContext(ctx, r.db).Where("account_key = ?", accountKey).Delete(&model.CredentialRecord{}).Error
}

func credentialFromRecord(record model.CredentialRecord) *biz.Credential {
	return &biz.Credential{
		BindingRef:        record.BindingRef,
		AccountKey:        record.AccountKey,
		Generation:        record.Generation,
		Platform:          record.Platform,
		AccountID:         record.AccountID,
		Region:            record.Region,
		CredentialBlob:    record.CredentialBlob,
		CredentialVersion: record.CredentialVersion,
		Status:            record.Status,
		LastValidatedAt:   record.LastValidatedAt,
		LastRefreshedAt:   record.LastRefreshedAt,
		ExpiresAt:         record.ExpiresAt,
		ProfileSnapshotComplete: record.ProfileSnapshotComplete,
		ProfileRevision:         record.ProfileRevision,
		ProfileObservedRevision: record.ProfileObservedRevision,
	}
}
