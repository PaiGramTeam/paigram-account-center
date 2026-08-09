package adminaudit

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"paigram/internal/handler"
	"paigram/internal/httpserver"
	"paigram/internal/middleware"
)

// RouterGroup holds phase-two admin audit routers.
type RouterGroup struct{}

// Init registers the phase-two /admin/audit-logs routes.
func (r *RouterGroup) Init(rg *gin.RouterGroup, _ *gorm.DB) {
	registerRoutes(r, rg)
}

func (r *RouterGroup) Register(rg *httpserver.Group, _ *gorm.DB) {
	registerContracts(rg)
	registerRoutes(r, rg)
}

func registerRoutes[T httpserver.RouteGroup[T]](_ *RouterGroup, rg T) {
	adminAudit := httpserver.WithAccess(rg.Group("/admin"), httpserver.Access{
		Authenticated: true, DynamicPermissions: []string{"casbin:path-and-method"}, RequiredRoles: []string{"admin"},
	})
	adminAudit.Use(middleware.RequireRoleMiddleware("admin"), middleware.CasbinMiddleware())
	{
		adminAudit.GET("/audit-logs", handler.ApiGroupApp.AdminAuditApiGroup.AuditHandler.ListAuditLogs)
		adminAudit.GET("/audit-logs/:id", handler.ApiGroupApp.AdminAuditApiGroup.AuditHandler.GetAuditLog)
	}
}
