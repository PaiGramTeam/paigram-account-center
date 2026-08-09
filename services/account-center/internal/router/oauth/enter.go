package oauth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"paigram/internal/handler"
	handleroauth "paigram/internal/handler/oauth"
	"paigram/internal/httpserver"
	"paigram/internal/middleware"
	"paigram/internal/service/credentials"
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
	registerPublic(r, rg)
}

func (r *RouterGroup) RegisterPublic(rg *httpserver.Group) {
	tokenContract := httpserver.FormContract(
		handleroauth.TokenRequest{}, credentials.IssuedToken{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusInternalServerError,
	).WithoutBodyValidation().WithErrorResponse(
		handleroauth.TokenErrorResponse{}, http.StatusBadRequest, http.StatusUnauthorized, http.StatusInternalServerError,
	)
	rg.RegisterContract(http.MethodPost, "/oauth/token", tokenContract)
	registerPublic(r, rg)
}

func registerPublic[T httpserver.RouteGroup[T]](_ *RouterGroup, rg T) {
	tokenHandler := &handler.ApiGroupApp.OAuthApiGroup.TokenHandler
	rg.POST("/oauth/token", tokenHandler.Token)
}

// InitAdmin mounts the admin CRUD endpoints behind the admin-role gate
// and the casbin permission check.
func (r *RouterGroup) InitAdmin(rg *gin.RouterGroup) {
	registerAdmin(r, rg)
}

func (r *RouterGroup) RegisterAdmin(rg *httpserver.Group) {
	registerAdminContracts(rg)
	registerAdmin(r, rg)
}

func registerAdmin[T httpserver.RouteGroup[T]](_ *RouterGroup, rg T) {
	adminGate := middleware.RequireRoleMiddleware("admin")
	permissionCheck := middleware.CasbinMiddleware()
	credentialsHandler := &handler.ApiGroupApp.OAuthApiGroup.CredentialsHandler

	credentialsAdmin := httpserver.WithAccess(rg.Group("/admin/service-credentials"), httpserver.Access{
		Authenticated: true, DynamicPermissions: []string{"casbin:path-and-method"}, RequiredRoles: []string{"admin"},
	})
	credentialsAdmin.Use(adminGate, permissionCheck)
	{
		credentialsAdmin.GET("", credentialsHandler.List)
		credentialsAdmin.POST("", credentialsHandler.Create)
		credentialsAdmin.POST("/:client_id/secret", credentialsHandler.RotateSecret)
	}
}
