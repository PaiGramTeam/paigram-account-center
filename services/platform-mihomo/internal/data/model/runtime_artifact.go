package model

import "time"

type RuntimeArtifact struct {
	ID                uint64    `gorm:"primaryKey"`
	BindingRef        string    `gorm:"not null;uniqueIndex:uniq_runtime_artifact_binding,priority:1;index:idx_runtime_binding_ref"`
	AccountKey        string    `gorm:"size:64;not null;index:idx_runtime_account_key"`
	ArtifactType      string    `gorm:"size:64;not null;uniqueIndex:uniq_runtime_artifact_binding,priority:2;index:idx_runtime_artifact_type"`
	ArtifactValue     string    `gorm:"type:text;not null"`
	ScopeKey          string    `gorm:"size:128;not null;uniqueIndex:uniq_runtime_artifact_binding,priority:3"`
	ExpiresAt         time.Time `gorm:"not null;index:idx_runtime_expires_at"`
	RevocationPending bool      `gorm:"not null"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
