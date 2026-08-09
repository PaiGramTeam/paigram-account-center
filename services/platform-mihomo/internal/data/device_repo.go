package data

import (
	"context"

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
		BindingID:         device.BindingID,
		PlatformAccountID: device.PlatformAccountID,
		DeviceID:          device.DeviceID,
		DeviceFP:          device.DeviceFP,
		DeviceName:        device.DeviceName,
		IsValid:           device.IsValid,
		LastSeenAt:        device.LastSeenAt,
	}

	return dbFromContext(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "binding_id"}, {Name: "device_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"platform_account_id", "device_fp", "device_name", "is_valid", "last_seen_at", "updated_at"}),
	}).Create(&record).Error
}

func (r *DeviceRepo) ListByBindingID(ctx context.Context, bindingID uint64) ([]*biz.Device, error) {
	var records []model.DeviceRecord
	if err := dbFromContext(ctx, r.db).Where("binding_id = ?", bindingID).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	return devicesFromRecords(records), nil
}

func (r *DeviceRepo) ListByPlatformAccountID(ctx context.Context, platformAccountID string) ([]*biz.Device, error) {
	var records []model.DeviceRecord
	if err := dbFromContext(ctx, r.db).Where("platform_account_id = ?", platformAccountID).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	return devicesFromRecords(records), nil
}

func (r *DeviceRepo) DeleteByBindingID(ctx context.Context, bindingID uint64) error {
	return dbFromContext(ctx, r.db).Where("binding_id = ?", bindingID).Delete(&model.DeviceRecord{}).Error
}

func (r *DeviceRepo) DeleteByPlatformAccountID(ctx context.Context, platformAccountID string) error {
	return dbFromContext(ctx, r.db).Where("platform_account_id = ?", platformAccountID).Delete(&model.DeviceRecord{}).Error
}

func devicesFromRecords(records []model.DeviceRecord) []*biz.Device {
	devices := make([]*biz.Device, 0, len(records))
	for _, record := range records {
		record := record
		devices = append(devices, &biz.Device{
			BindingID:         record.BindingID,
			PlatformAccountID: record.PlatformAccountID,
			DeviceID:          record.DeviceID,
			DeviceFP:          record.DeviceFP,
			DeviceName:        record.DeviceName,
			IsValid:           record.IsValid,
			LastSeenAt:        record.LastSeenAt,
		})
	}

	return devices
}
