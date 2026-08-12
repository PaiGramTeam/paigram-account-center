package model

import "time"

type CredentialRecord struct {
	ID                uint64 `gorm:"primaryKey"`
	BindingRef        string `gorm:"not null;uniqueIndex:uniq_credential_binding_ref"`
	AccountKey        string `gorm:"size:64;not null;uniqueIndex:uniq_account_key"`
	Generation        uint64 `gorm:"not null"`
	Platform          string `gorm:"size:32;not null;uniqueIndex:uniq_platform_account,priority:1"`
	AccountID         string `gorm:"size:64;not null;uniqueIndex:uniq_platform_account,priority:2"`
	Region            string `gorm:"size:32;not null"`
	CredentialBlob    string `gorm:"type:text;not null"`
	CredentialVersion string `gorm:"size:32;not null"`
	Status            string `gorm:"size:32;not null"`
	LastValidatedAt   *time.Time
	LastRefreshedAt   *time.Time
	ExpiresAt         *time.Time
	ProfileSnapshotComplete bool `gorm:"not null"`
	ProfileRevision         uint64 `gorm:"not null"`
	ProfileObservedRevision uint64 `gorm:"not null"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
