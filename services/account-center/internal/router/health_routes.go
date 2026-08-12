package router

import (
	"net/http"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/servicehealth"
	"github.com/gin-gonic/gin"

	"paigram/internal/response"
)

func registerHealthRoutes(engine *gin.Engine, readiness servicehealth.Checker) {
	live := func(c *gin.Context) {
		response.Success(c, gin.H{"status": "ok"})
	}
	ready := func(c *gin.Context) {
		if readiness == nil || readiness.Check(c.Request.Context()) != nil {
			response.Error(c, http.StatusServiceUnavailable, "not ready")
			return
		}
		response.Success(c, gin.H{"status": "ok"})
	}

	engine.GET("/livez", live)
	engine.GET("/readyz", ready)
	engine.GET("/healthz", ready)
}
