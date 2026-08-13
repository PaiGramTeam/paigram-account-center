package model

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BotIdentity maps a bot-specific external user to a unified account-center user.
type BotIdentity struct {
	ID               uint64         `gorm:"primaryKey"`
	EntryIdentityRef string         `gorm:"size:64;not null;uniqueIndex:uk_bot_identities_ref"`
	EntryEpoch       uint64         `gorm:"not null;default:1"`
	UserID           uint64         `gorm:"not null;index"`
	BotID            string         `gorm:"size:64;not null"`
	Issuer           string         `gorm:"size:191;not null;index"`
	ExternalUserID   string         `gorm:"size:191;not null"`
	ExternalUsername sql.NullString `gorm:"size:255"`
	LinkedAt         time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	CreatedAt        time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt        time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`

	User User `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Bot  Bot  `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:BotID;references:ID"`
}

func (identity *BotIdentity) BeforeCreate(_ *gorm.DB) error {
	if identity.EntryIdentityRef == "" {
		identity.EntryIdentityRef = "entry_" + uuid.NewString()
	}
	if identity.EntryEpoch == 0 {
		identity.EntryEpoch = 1
	}
	if identity.Issuer == "" {
		identity.Issuer = DefaultEntryIssuer(identity.BotID)
	}
	return nil
}

func (BotIdentity) TableName() string {
	return "bot_identities"
}
