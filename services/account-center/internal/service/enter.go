package service

import (
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
// The credentials TokenService is constructed with a fixed 32-byte
// placeholder signing key here because main-line server initialisation
// passes the configured SHARED_TICKET_KEY via the handler-side
// InitializeApiGroups path. This constructor exists only for tests
// that need a bare ServiceGroup; in production server startup
// (cmd/paigram/cmd/serve.go) the configured key is plumbed through the
// handler-side initialiser instead. The placeholder key must still
// satisfy credentials.NewTokenService's ≥32-byte minimum.
func NewServiceGroup(db *gorm.DB) *ServiceGroup {
	casbinGroup := casbin.NewServiceGroup(db)
	platformGroup := platform.NewServiceGroup(db)
	// 32 bytes of low-entropy ASCII — sufficient only for the constructor
	// guard; production wiring replaces this group via InitializeApiGroups.
	placeholderSigningKey := []byte("placeholder-32byte-test-key-xxxx")
	credentialsGroup, err := credentials.NewServiceGroup(db, credentials.TokenServiceConfig{
		SigningKey: placeholderSigningKey,
	})
	if err != nil {
		// Programmer error: the placeholder above is statically known to
		// satisfy the 32-byte minimum. Panic rather than propagating
		// because this constructor has no error return and callers (tests)
		// would not handle one usefully.
		panic("service.NewServiceGroup: placeholder signing key rejected: " + err.Error())
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
