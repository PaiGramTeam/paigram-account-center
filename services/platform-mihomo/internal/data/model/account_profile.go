package model

import "time"

type AccountProfile struct {
	ID                   uint64    `gorm:"primaryKey"`
	BindingRef           string    `gorm:"not null;uniqueIndex:uniq_profile_binding_player_region,priority:1;uniqueIndex:uniq_default_profile_per_binding,priority:1;index:idx_profile_binding_ref"`
	AccountKey           string    `gorm:"size:64;not null;index:idx_profile_account_key"`
	ProfileRef           string    `gorm:"size:64;not null;uniqueIndex:uniq_profile_ref"`
	GameBiz              string    `gorm:"size:64;not null"`
	Region               string    `gorm:"size:32;not null;uniqueIndex:uniq_profile_binding_player_region,priority:3"`
	PlayerID             string    `gorm:"size:64;not null;uniqueIndex:uniq_profile_binding_player_region,priority:2"`
	Nickname             string    `gorm:"size:255;not null"`
	Level                int       `gorm:"not null;default:0"`
	IsDefault            bool      `gorm:"not null;default:false"`
	DefaultProfileMarker *int      `gorm:"->;type:smallint generated always as (case when is_default then 1 else null end) stored;uniqueIndex:uniq_default_profile_per_binding,priority:2"`
	DiscoveredAt         time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt            time.Time
}
