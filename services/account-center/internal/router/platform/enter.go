package platform

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"paigram/internal/handler"
	"paigram/internal/httpserver"
)

// RouterGroup holds platform-related routers.
type RouterGroup struct{}

// Init registers platform routes on the provided router group.
func (r *RouterGroup) Init(rg *gin.RouterGroup, _ *gorm.DB) {
	registerRoutes(r, rg)
}

func (r *RouterGroup) Register(rg *httpserver.Group, _ *gorm.DB) {
	registerRoutes(r, rg)
}

func registerRoutes[T httpserver.RouteGroup[T]](_ *RouterGroup, rg T) {
	platformHandler := &handler.ApiGroupApp.PlatformApiGroup.Handler

	me := rg.Group("/me")
	{
		me.GET("/platforms", platformHandler.ListPlatforms)
		me.GET("/platforms/:platform/schema", platformHandler.GetPlatformSchema)
	}
}
