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

// ServiceCredential is the OAuth 2.0 client_credentials (RFC 6749 §4.4)
// registry row. It replaces the f27c395-era machine_identities +
// machine_identity_secrets + machine_tokens + signing_keys quartet with a
// single bcrypt-secret-per-row record.
//
// `client_id` is human-readable (e.g. "telegram-service", "paigram-genshin")
// and serves both as the OAuth client identifier AND as the consumer name
// that consumer_grants rows reference (per Path D Option D — consumer ==
// client_id, no extra indirection table).
type ServiceCredential struct {
	ClientID    string         `gorm:"primaryKey;size:96"                       json:"client_id"`
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
}

// TableName pins the GORM table name; without it GORM would use the
// pluralized "service_credentials" which happens to be what we want, but
// pinning it makes the intent explicit and survives any naming-strategy
// drift.
func (ServiceCredential) TableName() string {
	return "service_credentials"
}
