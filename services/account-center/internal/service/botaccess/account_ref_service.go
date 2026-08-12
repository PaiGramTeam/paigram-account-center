package botaccess

import (
	"database/sql"
	"fmt"

	"gorm.io/gorm"

	"paigram/internal/model"
)

type AccountRefService struct {
	db *gorm.DB
}

func (s *AccountRefService) ResolveBotUser(botID, externalUserID string) (*model.BotIdentity, error) {
	var identity model.BotIdentity
	if err := s.db.Where("bot_id = ? AND external_user_id = ?", botID, externalUserID).First(&identity).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrBotIdentityNotFound
		}
		return nil, fmt.Errorf("resolve bot user: %w", err)
	}

	return &identity, nil
}

func (s *AccountRefService) ListAccessibleBindings(botID, externalUserID, platform string) ([]model.PlatformAccountBinding, error) {
	consumer, err := legacyConsumerForBotID(botID)
	if err != nil {
		return nil, err
	}
	return s.ListAccessibleBindingsForConsumer(botID, consumer, externalUserID, platform)
}

func (s *AccountRefService) ListAccessibleBindingsForConsumer(botID, consumer, externalUserID, platform string) ([]model.PlatformAccountBinding, error) {
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

func (s *AccountRefService) GetGrantedBinding(botID, externalUserID string, bindingID, profileID uint64) (*model.BotIdentity, *model.PlatformAccountBinding, *model.ConsumerGrant, error) {
	consumer, err := legacyConsumerForBotID(botID)
	if err != nil {
		return nil, nil, nil, err
	}
	return s.GetGrantedBindingForConsumer(botID, consumer, externalUserID, bindingID, profileID)
}

func (s *AccountRefService) GetGrantedBindingForConsumer(botID, consumer, externalUserID string, bindingID, profileID uint64) (*model.BotIdentity, *model.PlatformAccountBinding, *model.ConsumerGrant, error) {
	identity, err := s.ResolveBotUser(botID, externalUserID)
	if err != nil {
		return nil, nil, nil, err
	}
	if consumer == "" {
		return nil, nil, nil, ErrConsumerNotSupported
	}

	var binding model.PlatformAccountBinding
	if err := s.db.Where("id = ? AND owner_user_id = ?", bindingID, identity.UserID).First(&binding).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, nil, ErrPlatformAccountMissing
		}
		return nil, nil, nil, fmt.Errorf("get platform account binding: %w", err)
	}
	if binding.Status != model.PlatformAccountBindingStatusActive {
		return nil, nil, nil, ErrInactiveAccountRef
	}

	var grant model.ConsumerGrant
	if err := s.db.Where("binding_id = ? AND consumer = ?", binding.ID, consumer).First(&grant).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, nil, ErrBotGrantNotFound
		}
		return nil, nil, nil, fmt.Errorf("get consumer grant: %w", err)
	}
	if grant.Status != model.ConsumerGrantStatusActive || grant.RevokedAt.Valid {
		return nil, nil, nil, ErrBotGrantRevoked
	}
	if profileID != 0 {
		var profile model.PlatformAccountProfile
		if err := s.db.Where("binding_id = ?", binding.ID).First(&profile, profileID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, nil, nil, ErrPlatformAccountMissing
			}
			return nil, nil, nil, fmt.Errorf("get platform account profile: %w", err)
		}
	}

	return identity, &binding, &grant, nil
}

func (s *AccountRefService) GetGrantedScopes(botID string, bindingID uint64) ([]string, error) {
	consumer, err := legacyConsumerForBotID(botID)
	if err != nil {
		return nil, err
	}
	return s.GetGrantedScopesForConsumer(consumer, bindingID)
}

func (s *AccountRefService) GetGrantedScopesForConsumer(consumer string, bindingID uint64) ([]string, error) {
	if consumer == "" {
		return nil, ErrConsumerNotSupported
	}

	var grant model.ConsumerGrant
	if err := s.db.Where("binding_id = ? AND consumer = ?", bindingID, consumer).First(&grant).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrBotGrantNotFound
		}
		return nil, fmt.Errorf("get consumer grant scopes: %w", err)
	}
	if grant.Status != model.ConsumerGrantStatusActive || grant.RevokedAt.Valid {
		return nil, ErrBotGrantRevoked
	}

	scopes, err := DecodeGrantScopes(grant)
	if err != nil {
		return nil, fmt.Errorf("decode consumer grant scopes: %w", err)
	}

	return scopes, nil
}

// legacyConsumerForBotID returns the consumer string for the legacy REST
// surfaces (ListAccessibleBindings, GetGrantedBinding, GetGrantedScopes)
// that still receive bot_id from the caller rather than the
// CredentialClaims.ClientID populated by the OAuth interceptor.
//
// Under Path D Option D (spec §10) the consumer IS the calling
// credential's client_id, not the bot_id. This helper preserves the
// bot_id-as-consumer convention used by pre-Path-D test seeds and
// pre-migration REST callers. Operators MUST register service
// credentials whose client_id matches the bot_id for these legacy
// routes to resolve correctly.
//
// Deprecated: callers should migrate to the *ForConsumer variants which
// take a consumer string derived from CredentialClaimsFromContext.
// Tracked in Path D Phase 2 follow-up.
func legacyConsumerForBotID(botID string) (string, error) {
	if botID == "" {
		return "", ErrConsumerNotSupported
	}
	return botID, nil
}

func nullableBindingExternalAccountKey(value sql.NullString) string {
	if !value.Valid {
		return ""
	}

	return value.String
}
