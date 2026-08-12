package model

import (
	"database/sql"
	"time"

	"gorm.io/gorm"
)

// BotIdentity maps a bot-specific external user to a unified account-center user.
type BotIdentity struct {
	ID               uint64         `gorm:"primaryKey"`
	UserID           uint64         `gorm:"uniqueIndex:uk_bot_identities_user_bot,priority:1;not null;index"`
	BotID            string         `gorm:"size:64;uniqueIndex:uk_bot_identities_bot_external,priority:1;uniqueIndex:uk_bot_identities_user_bot,priority:2;not null"`
	ExternalUserID   string         `gorm:"size:191;uniqueIndex:uk_bot_identities_bot_external,priority:2;not null"`
	ExternalUsername sql.NullString `gorm:"size:255"`
	LinkedAt         time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	CreatedAt        time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt        time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`

	User User `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Bot  Bot  `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:BotID;references:ID"`
}

func (BotIdentity) TableName() string {
	return "bot_identities"
}
