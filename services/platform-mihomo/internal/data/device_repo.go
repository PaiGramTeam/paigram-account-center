package data

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data/model"
)

type DeviceRepo struct {
	db *gorm.DB
}

func NewDeviceRepo(db *gorm.DB) *DeviceRepo {
	return &DeviceRepo{db: db}
}

func (r *DeviceRepo) Save(ctx context.Context, device *biz.Device) error {
	record := model.DeviceRecord{
		BindingRef: device.BindingRef,
		AccountKey: device.AccountKey,
		DeviceRef:  device.DeviceRef,
		DeviceID:   device.DeviceID,
		DeviceFP:   device.DeviceFP,
		DeviceName: device.DeviceName,
		IsValid:    device.IsValid,
		LastSeenAt: device.LastSeenAt,
	}

	return dbFromContext(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "binding_ref"}, {Name: "device_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"account_key", "device_ref", "device_fp", "device_name", "is_valid", "last_seen_at", "updated_at"}),
	}).Create(&record).Error
}

func (r *DeviceRepo) GetByDeviceRef(ctx context.Context, bindingRef string, deviceRef string) (*biz.Device, error) {
	var record model.DeviceRecord
	err := dbFromContext(ctx, r.db).Where("binding_ref = ? AND device_ref = ?", bindingRef, deviceRef).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return deviceFromRecord(record), nil
}

func (r *DeviceRepo) ListByBindingRef(ctx context.Context, bindingRef string) ([]*biz.Device, error) {
	var records []model.DeviceRecord
	if err := dbFromContext(ctx, r.db).Where("binding_ref = ?", bindingRef).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	return devicesFromRecords(records), nil
}

func (r *DeviceRepo) ListByAccountKey(ctx context.Context, accountKey string) ([]*biz.Device, error) {
	var records []model.DeviceRecord
	if err := dbFromContext(ctx, r.db).Where("account_key = ?", accountKey).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	return devicesFromRecords(records), nil
}

func (r *DeviceRepo) DeleteByBindingRef(ctx context.Context, bindingRef string) error {
	return dbFromContext(ctx, r.db).Where("binding_ref = ?", bindingRef).Delete(&model.DeviceRecord{}).Error
}

func (r *DeviceRepo) DeleteByAccountKey(ctx context.Context, accountKey string) error {
	return dbFromContext(ctx, r.db).Where("account_key = ?", accountKey).Delete(&model.DeviceRecord{}).Error
}

func devicesFromRecords(records []model.DeviceRecord) []*biz.Device {
	devices := make([]*biz.Device, 0, len(records))
	for _, record := range records {
		record := record
		devices = append(devices, deviceFromRecord(record))
	}

	return devices
}

func deviceFromRecord(record model.DeviceRecord) *biz.Device {
	return &biz.Device{
		BindingRef: record.BindingRef,
		AccountKey: record.AccountKey,
		DeviceRef:  record.DeviceRef,
		DeviceID:   record.DeviceID,
		DeviceFP:   record.DeviceFP,
		DeviceName: record.DeviceName,
		IsValid:    record.IsValid,
		LastSeenAt: record.LastSeenAt,
	}
}
