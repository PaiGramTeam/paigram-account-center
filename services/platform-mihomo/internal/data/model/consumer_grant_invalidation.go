package model

import "time"

type ConsumerGrantInvalidation struct {
	ID                  uint64 `gorm:"primaryKey"`
	BindingID           uint64 `gorm:"not null;uniqueIndex:uk_consumer_grant_invalidations_binding_consumer,priority:1"`
	Consumer            string `gorm:"size:64;not null;uniqueIndex:uk_consumer_grant_invalidations_binding_consumer,priority:2"`
	MinimumGrantVersion uint64 `gorm:"not null"`
	InvalidatedAt       time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
