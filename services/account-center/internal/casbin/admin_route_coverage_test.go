package casbin_test

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	internalcasbin "paigram/internal/casbin"
	"paigram/internal/model"
	routeradmin "paigram/internal/router/admin"
	routeradminaudit "paigram/internal/router/adminaudit"
	routeradminsystem "paigram/internal/router/adminsystem"
	routeroauth "paigram/internal/router/oauth"
	routerplatformbinding "paigram/internal/router/platformbinding"
)

func TestAdministratorPoliciesCoverEveryRegisteredAdminRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")

	(&routeradmin.RouterGroup{}).Init(v1, nil)
	(&routeradminaudit.RouterGroup{}).Init(v1, nil)
	(&routeradminsystem.RouterGroup{}).Init(v1, nil)
	(&routerplatformbinding.RouterGroup{}).Init(v1, nil)
	(&routeroauth.RouterGroup{}).InitAdmin(v1)

	covered := make(map[internalcasbin.PolicyRule]struct{})
	for _, rule := range internalcasbin.PoliciesForSystemRole(model.RoleAdmin) {
		covered[rule] = struct{}{}
	}

	for _, route := range engine.Routes() {
		if !strings.HasPrefix(route.Path, "/api/v1/admin/") {
			continue
		}
		rule := internalcasbin.PolicyRule{Path: route.Path, Method: route.Method}
		_, ok := covered[rule]
		assert.True(t, ok, "registered admin route is missing from the permission catalog: %s %s", route.Method, route.Path)
	}
}
