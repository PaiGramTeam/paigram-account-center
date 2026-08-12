package botaccess

import (
	"database/sql"
	"fmt"

	"gorm.io/gorm"

	"paigram/internal/model"
)

type BindingAccessService struct {
	db *gorm.DB
}

func (s *BindingAccessService) ResolveBotUser(botID, externalUserID string) (*model.BotIdentity, error) {
	return resolveBotUser(s.db, botID, externalUserID)
}

func resolveBotUser(db *gorm.DB, botID, externalUserID string) (*model.BotIdentity, error) {
	var identity model.BotIdentity
	if err := db.Where("bot_id = ? AND external_user_id = ?", botID, externalUserID).First(&identity).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrBotIdentityNotFound
		}
		return nil, fmt.Errorf("resolve bot user: %w", err)
	}

	return &identity, nil
}

func (s *BindingAccessService) ListAccessibleBindingsForConsumer(botID, consumer, externalUserID, platform string) ([]model.PlatformAccountBinding, error) {
	identity, err := s.ResolveBotUser(botID, externalUserID)
	if err != nil {
		return nil, err
	}
	if consumer == "" {
		return nil, ErrConsumerNotSupported
	}

	query := s.db.Model(&model.PlatformAccountBinding{}).
		Joins("JOIN consumer_grants ON consumer_grants.binding_id = platform_account_bindings.id").
		Where("platform_account_bindings.owner_user_id = ?", identity.UserID).
		Where("consumer_grants.consumer = ?", consumer).
		Where("consumer_grants.status = ?", model.ConsumerGrantStatusActive).
		Where("consumer_grants.revoked_at IS NULL").
		Where("platform_account_bindings.status = ?", model.PlatformAccountBindingStatusActive)

	if platform != "" {
		query = query.Where("platform_account_bindings.platform = ?", platform)
	}

	var bindings []model.PlatformAccountBinding
	if err := query.Order("platform_account_bindings.created_at ASC").Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("list accessible accounts: %w", err)
	}

	return bindings, nil
}

func (s *BindingAccessService) GetGrantedBindingByRefForConsumer(botID, consumer, externalUserID, bindingRef, profileRef string) (*model.BotIdentity, *model.PlatformAccountBinding, *model.ConsumerGrant, error) {
	if consumer == "" {
		return nil, nil, nil, ErrConsumerNotSupported
	}
	if bindingRef == "" {
		return nil, nil, nil, ErrPlatformAccountMissing
	}

	var identity *model.BotIdentity
	var binding model.PlatformAccountBinding
	var grant model.ConsumerGrant
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		identity, err = resolveBotUser(tx, botID, externalUserID)
		if err != nil {
			return err
		}
		if err := tx.Where("binding_ref = ? AND owner_user_id = ?", bindingRef, identity.UserID).First(&binding).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrPlatformAccountMissing
			}
			return fmt.Errorf("get platform account binding: %w", err)
		}
		if binding.Status != model.PlatformAccountBindingStatusActive {
			return ErrInactiveBinding
		}
		if err := tx.Preload("Actions").Where("binding_id = ? AND consumer = ?", binding.ID, consumer).First(&grant).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrConsumerGrantNotFound
			}
			return fmt.Errorf("get consumer grant: %w", err)
		}
		if grant.Status != model.ConsumerGrantStatusActive || grant.RevokedAt.Valid {
			return ErrConsumerGrantRevoked
		}
		if profileRef != "" {
			var count int64
			if err := tx.Model(&model.PlatformAccountProfile{}).Where("binding_id = ? AND profile_ref = ?", binding.ID, profileRef).Count(&count).Error; err != nil {
				return fmt.Errorf("get platform account profile: %w", err)
			}
			if count != 1 {
				return ErrPlatformAccountMissing
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, nil, nil, err
	}
	return identity, &binding, &grant, nil
}

func nullableBindingExternalAccountKey(value sql.NullString) string {
	if !value.Valid {
		return ""
	}

	return value.String
}
