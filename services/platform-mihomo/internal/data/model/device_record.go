package model

import "time"

type DeviceRecord struct {
	ID         uint64  `gorm:"primaryKey"`
	BindingRef string  `gorm:"not null;uniqueIndex:uniq_device_record_binding,priority:1;index:idx_device_binding_ref"`
	AccountKey string  `gorm:"size:64;not null;index:idx_device_account_key"`
	DeviceRef  string  `gorm:"size:64;not null;uniqueIndex:uniq_device_ref"`
	DeviceID   string  `gorm:"size:64;not null;uniqueIndex:uniq_device_record_binding,priority:2;index:idx_device_device_id"`
	DeviceFP   string  `gorm:"size:64;not null"`
	DeviceName *string `gorm:"size:128"`
	IsValid    bool    `gorm:"not null;default:true"`
	LastSeenAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
