package model

import (
	"database/sql"
	"time"
)

type EntryIdentityLinkChallengeStatus string

const (
	EntryIdentityLinkChallengePending   EntryIdentityLinkChallengeStatus = "pending"
	EntryIdentityLinkChallengeApproved  EntryIdentityLinkChallengeStatus = "approved"
	EntryIdentityLinkChallengeCancelled EntryIdentityLinkChallengeStatus = "cancelled"
	EntryIdentityLinkChallengeExpired   EntryIdentityLinkChallengeStatus = "expired"
	EntryIdentityLinkChallengeConflict  EntryIdentityLinkChallengeStatus = "conflict"
)

type EntryIdentityLinkChallenge struct {
	ChallengeHash    string                           `gorm:"primaryKey;size:64"`
	Consumer         string                           `gorm:"size:96;not null;index"`
	BotID            string                           `gorm:"size:64;not null;index"`
	Issuer           string                           `gorm:"size:191;not null"`
	ExternalSubject  string                           `gorm:"size:191;not null"`
	ExternalUsername sql.NullString                   `gorm:"size:255"`
	Status           EntryIdentityLinkChallengeStatus `gorm:"size:32;not null;index"`
	ExpiresAt        time.Time                        `gorm:"not null;index"`
	ApprovedUserID   sql.NullInt64                    `gorm:"type:bigint;index"`
	ConsumedAt       sql.NullTime                     `gorm:"type:timestamptz"`
	CreatedAt        time.Time                        `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt        time.Time                        `gorm:"not null;default:CURRENT_TIMESTAMP"`

	Bot          Bot               `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;foreignKey:BotID;references:ID"`
	Credential   ServiceCredential `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;foreignKey:Consumer;references:ClientID"`
	ApprovedUser *User             `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;foreignKey:ApprovedUserID;references:ID"`
}

func (EntryIdentityLinkChallenge) TableName() string {
	return "entry_identity_link_challenges"
}
