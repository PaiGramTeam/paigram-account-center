package oauth

import (
	"github.com/gin-gonic/gin"

	"paigram/internal/handler"
	"paigram/internal/middleware"
)

// RouterGroup wires the public OAuth 2.0 token endpoint plus the admin
// CRUD endpoints for service_credentials. Mounting matches the legacy
// machineidentity layout so client-side URLs stay stable across the
// Path D refactor.
type RouterGroup struct{}

// InitPublic mounts POST /oauth/token. The handler enforces RFC 6749
// §4.4.2 form-encoded bodies and emits §5.1/§5.2-shaped responses.
//
// The route is intentionally NOT registered in the auth interceptor's
// publicMethods map because the auth interceptor is gRPC-only; HTTP
// requests pass through gin-side middleware. Token issuance has its
// own (client_id, client_secret) authentication.
func (r *RouterGroup) InitPublic(rg *gin.RouterGroup) {
	tokenHandler := &handler.ApiGroupApp.OAuthApiGroup.TokenHandler
	rg.POST("/oauth/token", tokenHandler.Token)
}

// InitAdmin mounts the admin CRUD endpoints behind the admin-role gate
// and the casbin permission check.
func (r *RouterGroup) InitAdmin(rg *gin.RouterGroup) {
	adminGate := middleware.RequireRoleMiddleware("admin")
	permissionCheck := middleware.CasbinMiddleware()
	credentialsHandler := &handler.ApiGroupApp.OAuthApiGroup.CredentialsHandler

	credentialsAdmin := rg.Group("/admin/service-credentials")
	credentialsAdmin.Use(adminGate, permissionCheck)
	{
		credentialsAdmin.GET("", credentialsHandler.List)
		credentialsAdmin.POST("", credentialsHandler.Create)
		credentialsAdmin.POST("/:client_id/secret", credentialsHandler.RotateSecret)
	}
}
