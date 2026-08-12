package model

import "time"

type AuthorizationFence struct {
	ID                   uint64 `gorm:"primaryKey"`
	BindingRef           string `gorm:"size:64;not null;uniqueIndex:uniq_authorization_fence,priority:1"`
	ConsumerPrincipal    string `gorm:"size:128;not null;uniqueIndex:uniq_authorization_fence,priority:2"`
	MinimumGrantVersion  uint64 `gorm:"not null"`
	MinimumOwnerEpoch    uint64 `gorm:"not null"`
	MinimumConsumerEpoch uint64 `gorm:"not null"`
	MinimumEntryEpoch    uint64 `gorm:"not null"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
