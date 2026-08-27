package router

import (
	"gorm.io/gorm"

	"paigram/internal/config"
	"paigram/internal/httpserver"
	routerAdmin "paigram/internal/router/admin"
	routerAdminAudit "paigram/internal/router/adminaudit"
	routerAdminSystem "paigram/internal/router/adminsystem"
	routerAuthority "paigram/internal/router/authority"
	routerCasbin "paigram/internal/router/casbin"
	routerMe "paigram/internal/router/me"
	routerMeIdentities "paigram/internal/router/meidentities"
	routerOAuth "paigram/internal/router/oauth"
	routerPlatform "paigram/internal/router/platform"
	routerPlatformBinding "paigram/internal/router/platformbinding"
	routerUser "paigram/internal/router/user"
)

// RouterGroup aggregates all router groups.
type RouterGroup struct {
	AdminRouterGroup           routerAdmin.RouterGroup
	UserRouterGroup            routerUser.RouterGroup
	CasbinRouterGroup          routerCasbin.RouterGroup
	AuthorityRouterGroup       routerAuthority.RouterGroup
	MeRouterGroup              routerMe.RouterGroup
	AdminSystemRouterGroup     routerAdminSystem.RouterGroup
	AdminAuditRouterGroup      routerAdminAudit.RouterGroup
	PlatformRouterGroup        routerPlatform.RouterGroup
	PlatformBindingRouterGroup routerPlatformBinding.RouterGroup
	// OAuthRouterGroup serves POST /oauth/token (RFC 6749 §4.4) plus the
	// /admin/service-credentials CRUD. Replaces the pre-Path-D
	// MachineIdentityRouterGroup (which also served /.well-known/jwks.json
	// and /me/consumer-identities — both removed in Path D §3.1).
	OAuthRouterGroup routerOAuth.RouterGroup
	// MeIdentitiesRouterGroup serves /me/bot-identities on the
	// session-authenticated /me group via InitializeRouterGroups below.
	MeIdentitiesRouterGroup routerMeIdentities.RouterGroup
}

// RouterGroupApp is the global router instance.
var RouterGroupApp = new(RouterGroup)

// InitializeRouterGroups sets up all router groups with dependencies.
func InitializeRouterGroups(rg *httpserver.Group, db *gorm.DB, authCfg config.AuthConfig) {
	RouterGroupApp.MeRouterGroup.AuthConfig = authCfg

	// Initialize admin router group
	RouterGroupApp.AdminRouterGroup.Register(rg, db)

	// Initialize phase-two router groups
	RouterGroupApp.MeRouterGroup.Register(rg, db)
	RouterGroupApp.AdminSystemRouterGroup.Register(rg, db)
	RouterGroupApp.AdminAuditRouterGroup.Register(rg, db)

	// Bot entry identities use the same protected /me tree as other owner
	// resources.
	RouterGroupApp.MeIdentitiesRouterGroup.Register(rg, db)

	// Initialize platform router group
	RouterGroupApp.PlatformRouterGroup.Register(rg, db)

	// Initialize platform binding router group
	RouterGroupApp.PlatformBindingRouterGroup.Register(rg, db)

	// Initialize OAuth admin routes (token endpoint is mounted in
	// router.go::New on the public router because it has its own
	// (client_id, client_secret) authentication and must not pass
	// through the session-cookie middleware).
	RouterGroupApp.OAuthRouterGroup.RegisterAdmin(rg)
}
