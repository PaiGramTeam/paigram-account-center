package model

import "time"

type PlatformOperation struct {
	OperationID        string    `gorm:"size:128;primaryKey"`
	Kind               string    `gorm:"size:64;not null"`
	BindingRef         string    `gorm:"size:64;not null;index:idx_operation_binding_ref"`
	PreGeneration      uint64    `gorm:"not null"`
	TargetGeneration   uint64    `gorm:"not null"`
	RequestFingerprint string    `gorm:"size:64;not null"`
	ExecutionToken     string    `gorm:"size:64;not null"`
	LeaseExpiresAt     time.Time `gorm:"not null"`
	State              string    `gorm:"size:32;not null"`
	ReasonCode         string    `gorm:"size:128;not null;default:''"`
	AccountKey         string    `gorm:"size:64;not null;default:''"`
	CredentialStatus   string    `gorm:"size:32;not null;default:''"`
	SnapshotJSON       string    `gorm:"type:text;not null;default:'{}'"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
