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
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(withTx(ctx, tx))
	})
}

func (r *CredentialRepo) Save(ctx context.Context, credential *biz.Credential) error {
	record := credentialRecord(credential)

	return dbFromContext(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "binding_id"}},
		UpdateAll: true,
	}).Create(&record).Error
}

func (r *CredentialRepo) Create(ctx context.Context, credential *biz.Credential) error {
	record := credentialRecord(credential)
	if err := dbFromContext(ctx, r.db).Create(&record).Error; err != nil {
		return mapCredentialDuplicateError(err)
	}
	return nil
}

func credentialRecord(credential *biz.Credential) model.CredentialRecord {
	return model.CredentialRecord{
		BindingID:         credential.BindingID,
		PlatformAccountID: credential.PlatformAccountID,
		Platform:          credential.Platform,
		AccountID:         credential.AccountID,
		Region:            credential.Region,
		CredentialBlob:    credential.CredentialBlob,
		CredentialVersion: credential.CredentialVersion,
		Status:            credential.Status,
		LastValidatedAt:   credential.LastValidatedAt,
		LastRefreshedAt:   credential.LastRefreshedAt,
		ExpiresAt:         credential.ExpiresAt,
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

func (r *CredentialRepo) GetByBindingID(ctx context.Context, bindingID uint64) (*biz.Credential, error) {
	var record model.CredentialRecord
	if err := dbFromContext(ctx, r.db).Where("binding_id = ?", bindingID).Order("id asc").Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return credentialFromRecord(record), nil
}

func (r *CredentialRepo) GetByPlatformAccountID(ctx context.Context, platformAccountID string) (*biz.Credential, error) {
	var record model.CredentialRecord
	if err := dbFromContext(ctx, r.db).Where("platform_account_id = ?", platformAccountID).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return credentialFromRecord(record), nil
}

func (r *CredentialRepo) DeleteByPlatformAccountID(ctx context.Context, platformAccountID string) error {
	return dbFromContext(ctx, r.db).Where("platform_account_id = ?", platformAccountID).Delete(&model.CredentialRecord{}).Error
}

func credentialFromRecord(record model.CredentialRecord) *biz.Credential {
	return &biz.Credential{
		BindingID:         record.BindingID,
		PlatformAccountID: record.PlatformAccountID,
		Platform:          record.Platform,
		AccountID:         record.AccountID,
		Region:            record.Region,
		CredentialBlob:    record.CredentialBlob,
		CredentialVersion: record.CredentialVersion,
		Status:            record.Status,
		LastValidatedAt:   record.LastValidatedAt,
		LastRefreshedAt:   record.LastRefreshedAt,
		ExpiresAt:         record.ExpiresAt,
	}
}
