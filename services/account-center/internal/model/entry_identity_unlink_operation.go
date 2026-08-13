package model

import (
	"database/sql"
	"time"
)

type EntryIdentityUnlinkState string

const (
	EntryIdentityUnlinkPending  EntryIdentityUnlinkState = "PROPAGATION_PENDING"
	EntryIdentityUnlinkComplete EntryIdentityUnlinkState = "UNLINKED"
)

type EntryIdentityUnlinkOperation struct {
	OperationID       string                   `gorm:"size:64;primaryKey"`
	UserID            uint64                   `gorm:"not null;index"`
	BotID             string                   `gorm:"size:64;not null"`
	EntryIdentityRef  string                   `gorm:"size:64;not null"`
	MinimumEntryEpoch uint64                   `gorm:"not null"`
	State             EntryIdentityUnlinkState `gorm:"size:32;not null"`
	CompletedAt       sql.NullTime
	CreatedAt         time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt         time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (EntryIdentityUnlinkOperation) TableName() string {
	return "entry_identity_unlink_operations"
}

type EntryIdentityUnlinkTarget struct {
	OperationID string `gorm:"size:64;primaryKey"`
	GrantID     uint64 `gorm:"primaryKey"`
	ConfirmedAt sql.NullTime
}

func (EntryIdentityUnlinkTarget) TableName() string {
	return "entry_identity_unlink_targets"
}
