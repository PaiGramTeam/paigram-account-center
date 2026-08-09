package handler

import (
	"errors"

	"gorm.io/gorm"

	"paigram/internal/casbin"
	"paigram/internal/config"
	handlerAdminAudit "paigram/internal/handler/adminaudit"
	handlerAdminSystem "paigram/internal/handler/adminsystem"
	handlerAuth "paigram/internal/handler/auth"
	handlerAuthority "paigram/internal/handler/authority"
	handlerCasbin "paigram/internal/handler/casbin"
	handlerMe "paigram/internal/handler/me"
	handlerMeIdentities "paigram/internal/handler/meidentities"
	handlerOAuth "paigram/internal/handler/oauth"
	handlerPlatform "paigram/internal/handler/platform"
	handlerPlatformBinding "paigram/internal/handler/platformbinding"
	handlerTelegramOIDC "paigram/internal/handler/telegramoidc"
	handlerUser "paigram/internal/handler/user"
	"paigram/internal/logging"
	"paigram/internal/service"
	serviceAudit "paigram/internal/service/audit"
	serviceAuthority "paigram/internal/service/authority"
	serviceBotLink "paigram/internal/service/botlink"
	serviceBotRoute "paigram/internal/service/botroute"
	serviceCasbin "paigram/internal/service/casbin"
	serviceCredentials "paigram/internal/service/credentials"
	serviceGeolocation "paigram/internal/service/geolocation"
	serviceLoginRisk "paigram/internal/service/loginrisk"
	serviceMe "paigram/internal/service/me"
	servicePlatform "paigram/internal/service/platform"
	servicePlatformBinding "paigram/internal/service/platformbinding"
	serviceSession "paigram/internal/service/session"
	serviceSystemConfig "paigram/internal/service/systemconfig"
	serviceTelegramOIDC "paigram/internal/service/telegramoidc"
	serviceUser "paigram/internal/service/user"
	"paigram/internal/sessioncache"
)

// ApiGroup aggregates all API handler groups.
type ApiGroup struct {
	CasbinApiGroup          handlerCasbin.ApiGroup
	AuthApiGroup            handlerAuth.ApiGroup
	AuthorityApiGroup       handlerAuthority.ApiGroup
	PlatformApiGroup        handlerPlatform.ApiGroup
	PlatformBindingApiGroup handlerPlatformBinding.ApiGroup
	UserApiGroup            handlerUser.ApiGroup
	MeApiGroup              handlerMe.ApiGroup
	AdminSystemApiGroup     handlerAdminSystem.ApiGroup
	AdminAuditApiGroup      handlerAdminAudit.ApiGroup
	// OAuthApiGroup hosts the RFC 6749 §4.4 client_credentials token
	// endpoint plus the admin CRUD for service_credentials rows. It
	// replaces the pre-Path-D MachineIdentityApiGroup (Path D §3.1).
	OAuthApiGroup handlerOAuth.ApiGroup
	// TelegramOIDCApiGroup serves the Telegram OIDC login flow
	// (/auth/telegram/start + /auth/telegram/callback). See
	// docs/superpowers/specs/2026-06-06-phase5-sub1-telegram-oidc-bot-link.md §5.3.
	// Optional at runtime: nil OIDC pointer disables the wired routes (see
	// InitializeApiGroups below for the empty-config branch).
	TelegramOIDCApiGroup handlerTelegramOIDC.ApiGroup
	// MeIdentitiesApiGroup serves the authenticated user's view of their
	// linked Telegram identities (GET/DELETE /me/bot-identities). It reads
	// the same bot_identities table populated by the OIDC callback above
	// but does NOT itself require the OIDC client to be configured.
	MeIdentitiesApiGroup handlerMeIdentities.ApiGroup
}

// ApiGroupApp is the global API handler instance.
var ApiGroupApp = new(ApiGroup)

// InitializeApiGroups sets up all handler groups with dependencies.
//
// telegramOIDCCfg is opt-in: when all three credential fields are empty,
// the OIDC client is NOT constructed and the /auth/telegram/* routes
// become non-functional 500s. Deployments that don't use Telegram OIDC
// can leave the config block unset; they only need to validate it
// through config.Validate when they DO opt in. See spec §5.5.
func InitializeApiGroups(db *gorm.DB, cache sessioncache.Store, authCfg config.AuthConfig, securityCfg config.SecurityConfig, telegramOIDCCfg config.TelegramOIDCConfig) error {
	if db == nil {
		return errors.New("initialize api groups: db is nil")
	}

	// Initialize Casbin enforcer
	if _, err := casbin.InitEnforcer(db); err != nil {
		return err
	}

	// Initialize service layer
	service.ServiceGroupApp.UserServiceGroup = *serviceUser.NewServiceGroup(db)
	service.ServiceGroupApp.CasbinServiceGroup = *serviceCasbin.NewServiceGroup(db)
	service.ServiceGroupApp.AuthorityServiceGroup = *serviceAuthority.NewServiceGroup(db, &service.ServiceGroupApp.CasbinServiceGroup.CasbinService)
	service.ServiceGroupApp.MeServiceGroup = *serviceMe.NewServiceGroup(db, cache)
	service.ServiceGroupApp.SystemConfigGroup = *serviceSystemConfig.NewServiceGroup(db)
	service.ServiceGroupApp.AuditGroup = *serviceAudit.NewServiceGroup(db)
	service.ServiceGroupApp.PlatformServiceGroup = *servicePlatform.NewServiceGroup(db)
	if err := service.ServiceGroupApp.PlatformServiceGroup.PlatformService.ConfigureAuth(authCfg); err != nil {
		return err
	}
	service.ServiceGroupApp.PlatformServiceGroup.PlatformService.SetGenericSummaryProxy(servicePlatform.NewGRPCGenericSummaryProxy(nil))
	service.ServiceGroupApp.PlatformBindingGroup = *servicePlatformBinding.NewServiceGroup(db, &service.ServiceGroupApp.PlatformServiceGroup.PlatformService)
	service.ServiceGroupApp.BotRouteGroup = *serviceBotRoute.NewServiceGroup(db, nil)
	service.ServiceGroupApp.LoginRiskServiceGroup = *serviceLoginRisk.NewServiceGroup(db)
	service.ServiceGroupApp.GeolocationServiceGroup = *serviceGeolocation.NewServiceGroup()
	// Credentials service group: HS256 OAuth registry + token service.
	// The signing key is the deployment-wide SHARED_TICKET_KEY (Path D §1.4),
	// reused across access-token issuance and per-Dispatch service tickets.
	credentialsGroup, err := serviceCredentials.NewServiceGroup(db, serviceCredentials.TokenServiceConfig{
		Issuer:                authCfg.OAuthIssuer,
		AccessTokenTTLSeconds: authCfg.OAuthAccessTokenTTLSeconds,
		SigningKey:            []byte(authCfg.ServiceTicketSigningKey),
	})
	if err != nil {
		return err
	}
	service.ServiceGroupApp.CredentialsServiceGroup = *credentialsGroup

	// Initialize API handlers (passing db temporarily for non-refactored methods)
	ApiGroupApp.CasbinApiGroup = *handlerCasbin.NewApiGroup(&service.ServiceGroupApp.CasbinServiceGroup)
	ApiGroupApp.AuthorityApiGroup = *handlerAuthority.NewApiGroup(&service.ServiceGroupApp.AuthorityServiceGroup)
	ApiGroupApp.PlatformApiGroup = *handlerPlatform.NewApiGroup(&service.ServiceGroupApp.PlatformServiceGroup)
	ApiGroupApp.PlatformBindingApiGroup = *handlerPlatformBinding.NewApiGroup(&service.ServiceGroupApp.PlatformBindingGroup)
	ApiGroupApp.UserApiGroup = *handlerUser.NewApiGroup(&service.ServiceGroupApp.UserServiceGroup, db, cache, securityCfg)
	ApiGroupApp.MeApiGroup = *handlerMe.NewApiGroup(&service.ServiceGroupApp.MeServiceGroup)
	ApiGroupApp.AdminSystemApiGroup = *handlerAdminSystem.NewApiGroup(&service.ServiceGroupApp.SystemConfigGroup, &service.ServiceGroupApp.PlatformServiceGroup, &service.ServiceGroupApp.BotRouteGroup)
	ApiGroupApp.AdminAuditApiGroup = *handlerAdminAudit.NewApiGroup(&service.ServiceGroupApp.AuditGroup)
	ApiGroupApp.OAuthApiGroup = *handlerOAuth.NewApiGroup(&service.ServiceGroupApp.CredentialsServiceGroup)

	// Phase 5 Sub-project 1: Telegram OIDC login + bot identity linking.
	//
	// botlink + session are NEW packages (A3.1 / A4) that own their own
	// *gorm.DB handle; they are NOT exposed through service.ServiceGroupApp
	// because nothing else in the codebase consumes them today. Construct
	// them locally so the wiring chain stays self-contained.
	//
	// FindOrCreateOIDC is a method on the existing
	// service.ServiceGroupApp.UserServiceGroup.UserService (A3 added it
	// in-place); we pass that same instance so identity rows + user rows
	// stay coherent across the legacy /auth/oauth/* and the new
	// /auth/telegram/* paths.
	logger := logging.Logger()
	botlinkSvc := serviceBotLink.NewService(db, logger)
	sessionSvc := serviceSession.NewService(db, logger)
	ApiGroupApp.MeIdentitiesApiGroup = *handlerMeIdentities.NewApiGroup(botlinkSvc, logger)
	if telegramOIDCCfg.ClientID != "" || telegramOIDCCfg.ClientSecret != "" || telegramOIDCCfg.RedirectURI != "" {
		oidcClient := serviceTelegramOIDC.NewClient(serviceTelegramOIDC.Config{
			ClientID:     telegramOIDCCfg.ClientID,
			ClientSecret: telegramOIDCCfg.ClientSecret,
			RedirectURI:  telegramOIDCCfg.RedirectURI,
			// Test-only seam (A7): production configs leave the four
			// endpoint overrides empty so service/telegramoidc/config.go
			// applyDefaults pins them to oauth.telegram.org. Integration
			// tests inject a httptest.Server URL via in-process Config
			// literal construction; no file/env path reaches these
			// fields. See config.TelegramOIDCConfig godoc.
			AuthorizeEndpoint: telegramOIDCCfg.AuthorizeEndpoint,
			TokenEndpoint:     telegramOIDCCfg.TokenEndpoint,
			JWKSEndpoint:      telegramOIDCCfg.JWKSEndpoint,
			ExpectedIssuer:    telegramOIDCCfg.ExpectedIssuer,
		}, logger)
		stateStore := serviceTelegramOIDC.NewStateStore(db, logger)
		ApiGroupApp.TelegramOIDCApiGroup = *handlerTelegramOIDC.NewApiGroup(
			db,
			oidcClient,
			stateStore,
			&service.ServiceGroupApp.UserServiceGroup.UserService,
			sessionSvc,
			botlinkSvc,
			logger,
		)
	}

	return nil
}
