package botaccess

import (
	"database/sql"
	"testing"
	"time"

	"paigram/internal/model"
	"paigram/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBotAccessServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	return testutil.OpenPostgreSQLTestDB(t, "botaccess_service",
		&model.User{},
		&model.Bot{},
		&model.BotIdentity{},
		&model.PlatformService{},
		&model.PlatformAccountBinding{},
		&model.PlatformAccountProfile{},
		&model.ConsumerGrant{},
	)
}

func TestBindingAccessService_ResolveBotUser(t *testing.T) {
	db := setupBotAccessServiceTestDB(t)
	service := &BindingAccessService{db: db}

	user := model.User{PrimaryLoginType: model.LoginTypeOAuth, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&user).Error)

	bot := model.Bot{ID: "bot-resolve", DisplayName: "Resolve Bot", Type: "OTHER", Status: "ACTIVE", OwnerUserID: user.ID}
	require.NoError(t, db.Create(&bot).Error)

	identity := model.BotIdentity{
		UserID:           user.ID,
		BotID:            bot.ID,
		ExternalUserID:   "external-1",
		ExternalUsername: sql.NullString{String: "alice", Valid: true},
		LinkedAt:         time.Now().UTC(),
	}
	require.NoError(t, db.Create(&identity).Error)

	resolved, err := service.ResolveBotUser(bot.ID, identity.ExternalUserID)
	require.NoError(t, err)
	assert.Equal(t, identity.ID, resolved.ID)
	assert.Equal(t, user.ID, resolved.UserID)
	assert.Equal(t, "alice", resolved.ExternalUsername.String)

	missing, err := service.ResolveBotUser(bot.ID, "missing-user")
	require.ErrorIs(t, err, ErrBotIdentityNotFound)
	assert.Nil(t, missing)
}

func TestBindingAccessService_ListAccessibleBindingsFiltersByConsumerGrant(t *testing.T) {
	db := setupBotAccessServiceTestDB(t)
	service := &BindingAccessService{db: db}

	identity := seedBotIdentity(t, db, "bot-paigram", "external-list", 11)
	otherIdentity := seedBotIdentity(t, db, "bot-pamgram", "external-other", 12)
	otherBot := model.Bot{ID: "pamgram", DisplayName: "Other Same User", Type: "OTHER", Status: "ACTIVE", OwnerUserID: identity.UserID}
	require.NoError(t, db.Create(&otherBot).Error)
	require.NoError(t, db.Create(&model.BotIdentity{UserID: identity.UserID, BotID: otherBot.ID, ExternalUserID: "external-list-other-bot", LinkedAt: time.Now().UTC()}).Error)

	activeVisible := model.PlatformAccountBinding{
		OwnerUserID:        identity.UserID,
		Platform:           "telegram",
		ExternalAccountKey: sql.NullString{String: "acct-visible", Valid: true},
		PlatformServiceKey: "tg-main",
		DisplayName:        "Visible",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	filteredPlatform := model.PlatformAccountBinding{
		OwnerUserID:        identity.UserID,
		Platform:           "discord",
		ExternalAccountKey: sql.NullString{String: "acct-discord", Valid: true},
		PlatformServiceKey: "dc-main",
		DisplayName:        "Discord",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	inactive := model.PlatformAccountBinding{
		OwnerUserID:        identity.UserID,
		Platform:           "telegram",
		ExternalAccountKey: sql.NullString{String: "acct-inactive", Valid: true},
		PlatformServiceKey: "tg-main",
		DisplayName:        "Inactive",
		Status:             model.PlatformAccountBindingStatusDisabled,
	}
	noGrant := model.PlatformAccountBinding{
		OwnerUserID:        identity.UserID,
		Platform:           "telegram",
		ExternalAccountKey: sql.NullString{String: "acct-no-grant", Valid: true},
		PlatformServiceKey: "tg-main",
		DisplayName:        "No Grant",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	revoked := model.PlatformAccountBinding{
		OwnerUserID:        identity.UserID,
		Platform:           "telegram",
		ExternalAccountKey: sql.NullString{String: "acct-revoked", Valid: true},
		PlatformServiceKey: "tg-main",
		DisplayName:        "Revoked",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	otherOwner := model.PlatformAccountBinding{
		OwnerUserID:        otherIdentity.UserID,
		Platform:           "telegram",
		ExternalAccountKey: sql.NullString{String: "acct-other-owner", Valid: true},
		PlatformServiceKey: "tg-main",
		DisplayName:        "Other Owner",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&activeVisible).Error)
	require.NoError(t, db.Create(&filteredPlatform).Error)
	require.NoError(t, db.Create(&inactive).Error)
	require.NoError(t, db.Create(&noGrant).Error)
	require.NoError(t, db.Create(&revoked).Error)
	require.NoError(t, db.Create(&otherOwner).Error)

	consumer := "paigram-client"
	require.NoError(t, db.Create(&model.ConsumerGrant{BindingID: activeVisible.ID, Consumer: consumer, Status: model.ConsumerGrantStatusActive, GrantedAt: time.Now().UTC()}).Error)
	require.NoError(t, db.Create(&model.ConsumerGrant{BindingID: filteredPlatform.ID, Consumer: consumer, Status: model.ConsumerGrantStatusActive, GrantedAt: time.Now().UTC()}).Error)
	require.NoError(t, db.Create(&model.ConsumerGrant{BindingID: inactive.ID, Consumer: consumer, Status: model.ConsumerGrantStatusActive, GrantedAt: time.Now().UTC()}).Error)
	require.NoError(t, db.Create(&model.ConsumerGrant{BindingID: revoked.ID, Consumer: consumer, Status: model.ConsumerGrantStatusRevoked, GrantedAt: time.Now().UTC(), RevokedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}}).Error)
	require.NoError(t, db.Create(&model.ConsumerGrant{BindingID: otherOwner.ID, Consumer: consumer, Status: model.ConsumerGrantStatusActive, GrantedAt: time.Now().UTC()}).Error)

	accounts, err := service.ListAccessibleBindingsForConsumer(identity.BotID, consumer, identity.ExternalUserID, "telegram")
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	assert.Equal(t, activeVisible.ID, accounts[0].ID)
	assert.Equal(t, activeVisible.OwnerUserID, accounts[0].OwnerUserID)
	assert.Equal(t, "acct-visible", accounts[0].ExternalAccountKey.String)

	otherBotAccounts, err := service.ListAccessibleBindingsForConsumer(otherBot.ID, "pamgram-client", "external-list-other-bot", "telegram")
	require.NoError(t, err)
	assert.Empty(t, otherBotAccounts)
}

func TestBindingAccessService_GetGrantedBindingForConsumer(t *testing.T) {
	db := setupBotAccessServiceTestDB(t)
	service := &BindingAccessService{db: db}

	identity := seedBotIdentity(t, db, "bot-paigram", "external-grant", 31)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        identity.UserID,
		Platform:           "telegram",
		ExternalAccountKey: sql.NullString{String: "acct-lookup", Valid: true},
		PlatformServiceKey: "tg-main",
		DisplayName:        "Lookup",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&binding).Error)
	consumer := "paigram-client"
	grant := model.ConsumerGrant{BindingID: binding.ID, Consumer: consumer, Status: model.ConsumerGrantStatusActive, GrantedAt: time.Now().UTC()}
	require.NoError(t, db.Create(&grant).Error)

	resolvedIdentity, resolvedBinding, resolvedGrant, err := service.GetGrantedBindingForConsumer(identity.BotID, consumer, identity.ExternalUserID, binding.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, identity.ID, resolvedIdentity.ID)
	assert.Equal(t, binding.ID, resolvedBinding.ID)
	assert.Equal(t, binding.ID, resolvedGrant.BindingID)
}

func TestBindingAccessService_GetGrantedBindingForConsumerRejectsProfileFromOtherBinding(t *testing.T) {
	db := setupBotAccessServiceTestDB(t)
	service := &BindingAccessService{db: db}

	identity := seedBotIdentity(t, db, "bot-paigram", "external-grant-profile", 32)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        identity.UserID,
		Platform:           "mihomo",
		ExternalAccountKey: sql.NullString{String: "cn:binding", Valid: true},
		PlatformServiceKey: "platform-mihomo-service",
		DisplayName:        "Binding",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	otherBinding := model.PlatformAccountBinding{
		OwnerUserID:        identity.UserID,
		Platform:           "mihomo",
		ExternalAccountKey: sql.NullString{String: "cn:other-binding", Valid: true},
		PlatformServiceKey: "platform-mihomo-service",
		DisplayName:        "Other Binding",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&binding).Error)
	require.NoError(t, db.Create(&otherBinding).Error)
	consumer := "paigram-client"
	require.NoError(t, db.Create(&model.ConsumerGrant{BindingID: binding.ID, Consumer: consumer, Status: model.ConsumerGrantStatusActive, GrantedAt: time.Now().UTC()}).Error)
	foreignProfile := model.PlatformAccountProfile{
		BindingID:          otherBinding.ID,
		PlatformProfileKey: "mihomo:20002",
		GameBiz:            "hk4e_cn",
		Region:             "cn_gf01",
		PlayerUID:          "20002",
		Nickname:           "Foreign",
	}
	require.NoError(t, db.Create(&foreignProfile).Error)

	resolvedIdentity, resolvedBinding, resolvedGrant, err := service.GetGrantedBindingForConsumer(identity.BotID, consumer, identity.ExternalUserID, binding.ID, foreignProfile.ID)
	require.ErrorIs(t, err, ErrPlatformAccountMissing)
	assert.Nil(t, resolvedIdentity)
	assert.Nil(t, resolvedBinding)
	assert.Nil(t, resolvedGrant)
}

func TestBindingAccessService_GetGrantedScopesForConsumerReadsGrantScopes(t *testing.T) {
	db := setupBotAccessServiceTestDB(t)
	service := &BindingAccessService{db: db}

	identity := seedBotIdentity(t, db, "bot-paigram", "external-grant-scopes", 34)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        identity.UserID,
		Platform:           "telegram",
		ExternalAccountKey: sql.NullString{String: "acct-scope", Valid: true},
		PlatformServiceKey: "tg-main",
		DisplayName:        "Scoped",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&binding).Error)
	consumer := "paigram-client"
	require.NoError(t, db.Create(&model.ConsumerGrant{
		BindingID:  binding.ID,
		Consumer:   consumer,
		Status:     model.ConsumerGrantStatusActive,
		ScopesJSON: `["messages:read","messages:write"]`,
		GrantedAt:  time.Now().UTC(),
	}).Error)

	scopes, err := service.GetGrantedScopesForConsumer(consumer, binding.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"messages:read", "messages:write"}, scopes)
}

func TestBindingAccessService_ListAccessibleBindingsReturnsEmptyForBotWithoutGrants(t *testing.T) {
	db := setupBotAccessServiceTestDB(t)
	service := &BindingAccessService{db: db}

	identity := seedBotIdentity(t, db, "bot-unsupported", "external-unsupported", 33)

	// Consumer identity comes from the calling credential. A credential with
	// no matching grants receives an empty result.
	accounts, err := service.ListAccessibleBindingsForConsumer(identity.BotID, "unsupported-client", identity.ExternalUserID, "telegram")
	require.NoError(t, err)
	assert.Empty(t, accounts)
}

func seedBotIdentity(t *testing.T, db *gorm.DB, botID, externalUserID string, suffix uint64) model.BotIdentity {
	t.Helper()

	user := model.User{PrimaryLoginType: model.LoginTypeOAuth, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&user).Error)

	bot := model.Bot{
		ID:          botID,
		DisplayName: "Bot " + botID,
		Type:        "OTHER",
		Status:      "ACTIVE",
		OwnerUserID: user.ID,
	}
	require.NoError(t, db.Create(&bot).Error)

	identity := model.BotIdentity{
		UserID:           user.ID,
		BotID:            bot.ID,
		ExternalUserID:   externalUserID,
		ExternalUsername: sql.NullString{String: "user", Valid: true},
		LinkedAt:         time.Now().UTC().Add(time.Duration(suffix) * time.Second),
	}
	require.NoError(t, db.Create(&identity).Error)

	return identity
}
