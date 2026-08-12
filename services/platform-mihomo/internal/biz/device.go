package biz

import (
	"context"
	"time"
)

type Device struct {
	BindingRef string
	AccountKey string
	DeviceRef  string
	DeviceID   string
	DeviceFP   string
	DeviceName *string
	IsValid    bool
	LastSeenAt *time.Time
}

type DeviceRepository interface {
	Save(ctx context.Context, device *Device) error
	ListByBindingRef(ctx context.Context, bindingRef string) ([]*Device, error)
	ListByAccountKey(ctx context.Context, accountKey string) ([]*Device, error)
	GetByDeviceRef(ctx context.Context, bindingRef string, deviceRef string) (*Device, error)
	DeleteByBindingRef(ctx context.Context, bindingRef string) error
	DeleteByAccountKey(ctx context.Context, accountKey string) error
}
