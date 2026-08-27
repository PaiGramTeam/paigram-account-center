package service

import (
	"crypto/rand"

	serviceAudit "paigram/internal/service/audit"
	"paigram/internal/service/authority"
	"paigram/internal/service/botroute"
	"paigram/internal/service/casbin"
	"paigram/internal/service/credentials"
	"paigram/internal/service/geolocation"
	"paigram/internal/service/loginrisk"
	serviceMe "paigram/internal/service/me"
	"paigram/internal/service/platform"
	"paigram/internal/service/platformbinding"
	serviceSystemConfig "paigram/internal/service/systemconfig"
	"paigram/internal/service/user"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ServiceGroup aggregates all service groups.
type ServiceGroup struct {
	UserServiceGroup        user.ServiceGroup
	CasbinServiceGroup      casbin.ServiceGroup
	AuthorityServiceGroup   authority.ServiceGroup
	MeServiceGroup          serviceMe.ServiceGroup
	SystemConfigGroup       serviceSystemConfig.ServiceGroup
	AuditGroup              serviceAudit.ServiceGroup
	PlatformServiceGroup    platform.ServiceGroup
	PlatformBindingGroup    platformbinding.ServiceGroup
	BotRouteGroup           botroute.ServiceGroup
	LoginRiskServiceGroup   loginrisk.ServiceGroup
	GeolocationServiceGroup geolocation.ServiceGroup
	// CredentialsServiceGroup replaces the pre-Path-D MachineIdentity
	// service group: one HS256 client_credentials registry, no
	// asymmetric key registry, no per-JTI persistence (Path D §3.1).
	CredentialsServiceGroup credentials.ServiceGroup
}

// NewServiceGroup creates the global service group with all dependencies.
//
// This constructor exists for tests that need a bare ServiceGroup. Production
// startup replaces the credentials group through the handler initializer.
func NewServiceGroup(db *gorm.DB) *ServiceGroup {
	casbinGroup := casbin.NewServiceGroup(db)
	platformGroup := platform.NewServiceGroup(db)
	testSigningKey := make([]byte, 32)
	if _, err := rand.Read(testSigningKey); err != nil {
		panic("service.NewServiceGroup: generate test signing key: " + err.Error())
	}
	credentialsGroup, err := credentials.NewServiceGroup(db, credentials.TokenServiceConfig{
		SigningKey: testSigningKey,
	})
	if err != nil {
		panic("service.NewServiceGroup: test signing key rejected: " + err.Error())
	}
	return &ServiceGroup{
		UserServiceGroup:        *user.NewServiceGroup(db),
		CasbinServiceGroup:      *casbinGroup,
		AuthorityServiceGroup:   *authority.NewServiceGroup(db, &casbinGroup.CasbinService),
		MeServiceGroup:          *serviceMe.NewServiceGroup(db, nil),
		SystemConfigGroup:       *serviceSystemConfig.NewServiceGroup(db),
		AuditGroup:              *serviceAudit.NewServiceGroup(db),
		PlatformServiceGroup:    *platformGroup,
		PlatformBindingGroup:    *platformbinding.NewServiceGroup(db, &platformGroup.PlatformService),
		BotRouteGroup:           *botroute.NewServiceGroup(db, zap.L()),
		LoginRiskServiceGroup:   *loginrisk.NewServiceGroup(db),
		GeolocationServiceGroup: *geolocation.NewServiceGroup(),
		CredentialsServiceGroup: *credentialsGroup,
	}
}

// ServiceGroupApp is the global service instance.
var ServiceGroupApp = new(ServiceGroup)
