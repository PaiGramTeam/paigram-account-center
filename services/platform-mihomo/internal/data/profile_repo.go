package data

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data/model"
)

var ErrDefaultProfileAlreadyExists = errors.New("default profile already exists for binding")

type ProfileRepo struct {
	db *gorm.DB
}

func NewProfileRepo(db *gorm.DB) *ProfileRepo {
	return &ProfileRepo{db: db}
}

func (r *ProfileRepo) Save(ctx context.Context, profile *biz.Profile) error {
	db := dbFromContext(ctx, r.db)
	record := model.AccountProfile{
		BindingRef:   profile.BindingRef,
		AccountKey:   profile.AccountKey,
		ProfileRef:   profile.ProfileRef,
		GameBiz:      profile.GameBiz,
		Region:       profile.Region,
		PlayerID:     profile.PlayerID,
		Nickname:     profile.Nickname,
		Level:        profile.Level,
		IsDefault:    profile.IsDefault,
		DiscoveredAt: profile.DiscoveredAt,
	}
	if profile.IsDefault {
		return saveDefaultProfile(db, profile, &record)
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "binding_ref"}, {Name: "player_id"}, {Name: "region"}},
		DoUpdates: clause.AssignmentColumns([]string{"account_key", "profile_ref", "game_biz", "nickname", "level", "is_default", "updated_at"}),
	}).Create(&record).Error
}

func (r *ProfileRepo) SetDefaultByBindingAndPlayerID(ctx context.Context, bindingRef string, accountKey string, playerID string) error {
	return dbFromContext(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.AccountProfile{}).
			Where("binding_ref = ? AND is_default = ?", bindingRef, true).
			Update("is_default", false).Error; err != nil {
			return err
		}

		result := tx.Model(&model.AccountProfile{}).
			Where("binding_ref = ? AND account_key = ? AND player_id = ?", bindingRef, accountKey, playerID).
			Update("is_default", true)
		if err := mapDefaultProfileDuplicateError(result.Error); err != nil {
			return err
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func saveDefaultProfile(db *gorm.DB, profile *biz.Profile, record *model.AccountProfile) error {
	var existing model.AccountProfile
	err := db.Where("binding_ref = ? AND player_id = ? AND region = ?", profile.BindingRef, profile.PlayerID, profile.Region).
		Take(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if existing.ID != 0 {
		if err := ensureNoConflictingDefaultProfile(db, profile, existing.ID); err != nil {
			return err
		}
		return mapDefaultProfileDuplicateError(db.Model(&model.AccountProfile{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{
				"account_key": profile.AccountKey,
				"profile_ref": profile.ProfileRef,
				"game_biz":    profile.GameBiz,
				"nickname":    profile.Nickname,
				"level":       profile.Level,
				"is_default":  true,
			}).Error)
	}

	if err := ensureNoConflictingDefaultProfile(db, profile, 0); err != nil {
		return err
	}
	return mapDefaultProfileDuplicateError(db.Create(record).Error)
}

func ensureNoConflictingDefaultProfile(db *gorm.DB, profile *biz.Profile, excludeID uint64) error {
	query := db.Model(&model.AccountProfile{}).
		Where("binding_ref = ? AND is_default = ?", profile.BindingRef, true).
		Where("NOT (binding_ref = ? AND player_id = ? AND region = ?)", profile.BindingRef, profile.PlayerID, profile.Region)
	if excludeID != 0 {
		query = query.Where("id <> ?", excludeID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrDefaultProfileAlreadyExists
	}
	return nil
}

func mapDefaultProfileDuplicateError(err error) error {
	if err == nil {
		return nil
	}
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) && postgresErr.Code == "23505" && postgresErr.ConstraintName == "uniq_default_profile_per_binding" {
		return ErrDefaultProfileAlreadyExists
	}
	if strings.Contains(err.Error(), "default_profile_marker") || strings.Contains(err.Error(), "uniq_default_profile_per_binding") {
		return ErrDefaultProfileAlreadyExists
	}
	return err
}

func (r *ProfileRepo) ListByBindingRef(ctx context.Context, bindingRef string) ([]*biz.Profile, error) {
	var records []model.AccountProfile
	if err := dbFromContext(ctx, r.db).Where("binding_ref = ?", bindingRef).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	return profilesFromRecords(records), nil
}

func (r *ProfileRepo) GetByProfileRef(ctx context.Context, bindingRef string, profileRef string) (*biz.Profile, error) {
	var record model.AccountProfile
	err := dbFromContext(ctx, r.db).Where("binding_ref = ? AND profile_ref = ?", bindingRef, profileRef).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	profiles := profilesFromRecords([]model.AccountProfile{record})
	return profiles[0], nil
}

func (r *ProfileRepo) ListByAccountKey(ctx context.Context, accountKey string) ([]*biz.Profile, error) {
	var records []model.AccountProfile
	if err := dbFromContext(ctx, r.db).Where("account_key = ?", accountKey).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	return profilesFromRecords(records), nil
}

func (r *ProfileRepo) DeleteByAccountKey(ctx context.Context, accountKey string) error {
	return dbFromContext(ctx, r.db).Where("account_key = ?", accountKey).Delete(&model.AccountProfile{}).Error
}

func (r *ProfileRepo) DeleteMissingByBindingRef(ctx context.Context, bindingRef string, keep []biz.ProfileIdentity) error {
	query := dbFromContext(ctx, r.db).Where("binding_ref = ?", bindingRef)
	if len(keep) == 0 {
		return query.Delete(&model.AccountProfile{}).Error
	}

	clauses := make([]string, 0, len(keep))
	args := make([]any, 0, len(keep)*2)
	for _, identity := range keep {
		clauses = append(clauses, "(player_id = ? AND region = ?)")
		args = append(args, identity.PlayerID, identity.Region)
	}
	return query.Where("NOT ("+strings.Join(clauses, " OR ")+")", args...).Delete(&model.AccountProfile{}).Error
}

func (r *ProfileRepo) DeleteMissingByAccountKey(ctx context.Context, accountKey string, keep []biz.ProfileIdentity) error {
	var records []model.AccountProfile
	if err := dbFromContext(ctx, r.db).Where("account_key = ?", accountKey).Find(&records).Error; err != nil {
		return err
	}
	keepSet := make(map[string]struct{}, len(keep))
	for _, identity := range keep {
		keepSet[identity.PlayerID+":"+identity.Region] = struct{}{}
	}
	for _, record := range records {
		key := record.PlayerID + ":" + record.Region
		if _, ok := keepSet[key]; ok {
			continue
		}
		if err := dbFromContext(ctx, r.db).Delete(&model.AccountProfile{}, record.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

func profilesFromRecords(records []model.AccountProfile) []*biz.Profile {
	profiles := make([]*biz.Profile, 0, len(records))
	for _, record := range records {
		record := record
		profiles = append(profiles, &biz.Profile{
			ID:           record.ID,
			BindingRef:   record.BindingRef,
			AccountKey:   record.AccountKey,
			ProfileRef:   record.ProfileRef,
			GameBiz:      record.GameBiz,
			Region:       record.Region,
			PlayerID:     record.PlayerID,
			Nickname:     record.Nickname,
			Level:        record.Level,
			IsDefault:    record.IsDefault,
			DiscoveredAt: record.DiscoveredAt,
		})
	}

	return profiles
}
