package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// Bot is the thin identity record kept under Path D §10 Q5: it exists so
// `bot_identities (user_id, bot_id) → users.id` can FK-reference a stable
// bot identifier, decoupled from OAuth client credentials. A logical bot
// (e.g. "paigrambot") may be served by multiple service credentials
// (one per platform adapter — telegram-service, genshin-service, ...).
//
// Path D drops the legacy api_key / api_secret / scopes / metadata
// columns. Those moved to service_credentials per Path D §2.1; this row
// is purely an identity anchor for platform-user binding.
type Bot struct {
	ID          string         `gorm:"primaryKey;size:64"            json:"id"`
	EntryIssuer string         `gorm:"size:191;not null;uniqueIndex:uk_bots_entry_issuer" json:"entry_issuer"`
	DisplayName string         `gorm:"column:display_name;size:255;not null" json:"display_name"`
	Description string         `gorm:"type:text"                     json:"description"`
	Type        string         `gorm:"size:32;not null;default:'OTHER'" json:"type"`
	Status      string         `gorm:"size:32;not null;default:'ACTIVE';index" json:"status"`
	OwnerUserID uint64         `gorm:"index;not null"                json:"owner_user_id"`
	CreatedAt   time.Time      `                                     json:"created_at"`
	UpdatedAt   time.Time      `                                     json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"                         json:"-"`
}

func (bot *Bot) BeforeCreate(_ *gorm.DB) error {
	if strings.TrimSpace(bot.EntryIssuer) == "" {
		bot.EntryIssuer = DefaultEntryIssuer(bot.ID)
	}
	return nil
}

func DefaultEntryIssuer(botID string) string {
	return "urn:paigram:entry:" + strings.TrimSpace(botID)
}

// TableName pins the GORM table name to the migration's `bots`.
func (Bot) TableName() string {
	return "bots"
}
