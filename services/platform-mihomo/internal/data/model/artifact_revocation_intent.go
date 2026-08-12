package model

import "time"

type ArtifactRevocationIntent struct {
	IntentID       string     `gorm:"size:64;primaryKey"`
	BindingRef     string     `gorm:"size:64;not null;index:idx_artifact_revocation_binding"`
	AccountKey     string     `gorm:"size:64;not null"`
	ArtifactType   string     `gorm:"size:64;not null"`
	ArtifactValue  string     `gorm:"type:text;not null"`
	ScopeKey       string     `gorm:"size:128;not null"`
	ExpiresAt      time.Time  `gorm:"not null;index:idx_artifact_revocation_expiry"`
	State          string     `gorm:"size:16;not null;index:idx_artifact_revocation_ready"`
	ReadyAfter     time.Time  `gorm:"not null;index:idx_artifact_revocation_ready"`
	LeaseToken     *string    `gorm:"size:64"`
	LeaseExpiresAt *time.Time `gorm:"index:idx_artifact_revocation_lease"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
