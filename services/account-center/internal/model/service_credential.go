package model

import (
	"database/sql"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ServiceCredential statuses.
const (
	ServiceCredentialStatusActive   = "active"
	ServiceCredentialStatusDisabled = "disabled"
)

// ServiceCredential is an OAuth 2.0 client_credentials registry row.
// ClientID is the OAuth client identifier and consumer grant principal.
// BotID maps that service credential to the stable logical Bot identity.
type ServiceCredential struct {
	ClientID    string         `gorm:"primaryKey;size:96"                       json:"client_id"`
	BotID       string         `gorm:"size:64;not null;index"                   json:"bot_id"`
	DisplayName string         `gorm:"size:255;not null"                        json:"display_name"`
	SecretHash  string         `gorm:"size:255;not null"                        json:"-"`
	Audiences   datatypes.JSON `gorm:"type:jsonb;not null"                       json:"audiences"`
	Scopes      datatypes.JSON `gorm:"type:jsonb;not null"                       json:"scopes"`
	Status      string         `gorm:"size:32;index;not null;default:'active'"  json:"status"`
	OwnerUserID uint64         `gorm:"index;not null"                           json:"owner_user_id"`
	Description string         `gorm:"type:text"                                json:"description"`
	LastUsedAt  sql.NullTime   `gorm:"type:timestamptz"                         json:"-"`
	CreatedAt   time.Time      `                                                json:"created_at"`
	UpdatedAt   time.Time      `                                                json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"                                    json:"-"`

	Owner User `gorm:"foreignKey:OwnerUserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	Bot   Bot  `gorm:"foreignKey:BotID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
}

// TableName pins the GORM table name; without it GORM would use the
// pluralized "service_credentials" which happens to be what we want, but
// pinning it makes the intent explicit and survives any naming-strategy
// drift.
func (ServiceCredential) TableName() string {
	return "service_credentials"
}
