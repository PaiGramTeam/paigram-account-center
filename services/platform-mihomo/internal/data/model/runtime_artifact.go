package model

import "time"

type RuntimeArtifact struct {
	ID                uint64    `gorm:"primaryKey"`
	BindingID         uint64    `gorm:"not null;uniqueIndex:uniq_runtime_artifact_binding,priority:1;index:idx_runtime_binding_id"`
	PlatformAccountID string    `gorm:"size:64;not null;index:idx_runtime_platform_account_id"`
	ArtifactType      string    `gorm:"size:64;not null;uniqueIndex:uniq_runtime_artifact_binding,priority:2;index:idx_runtime_artifact_type"`
	ArtifactValue     string    `gorm:"type:text;not null"`
	ScopeKey          string    `gorm:"size:128;not null;uniqueIndex:uniq_runtime_artifact_binding,priority:3"`
	ExpiresAt         time.Time `gorm:"not null;index:idx_runtime_expires_at"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
