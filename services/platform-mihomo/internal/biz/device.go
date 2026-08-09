package biz

import (
	"context"
	"time"
)

type Device struct {
	BindingID         uint64
	PlatformAccountID string
	DeviceID          string
	DeviceFP          string
	DeviceName        *string
	IsValid           bool
	LastSeenAt        *time.Time
}

type DeviceRepository interface {
	Save(ctx context.Context, device *Device) error
	ListByBindingID(ctx context.Context, bindingID uint64) ([]*Device, error)
	ListByPlatformAccountID(ctx context.Context, platformAccountID string) ([]*Device, error)
	DeleteByBindingID(ctx context.Context, bindingID uint64) error
	DeleteByPlatformAccountID(ctx context.Context, platformAccountID string) error
}
