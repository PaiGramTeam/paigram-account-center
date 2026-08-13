// Package meidentities wires bot identity routes onto the authenticated user API.
package meidentities

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"paigram/internal/handler"
	handlermeidentities "paigram/internal/handler/meidentities"
	"paigram/internal/httpserver"
	"paigram/internal/response"
)

// RouterGroup wires the meidentities handler onto a router subtree.
type RouterGroup struct{}

// Init mounts the /me/bot-identities subtree on the SESSION-AUTHENTICATED
// group passed in by router.InitializeRouterGroups.
//
// The db argument is unused — kept in the signature so the wiring matches
// the sibling Init(rg, db) shape that the aggregating
// router.InitializeRouterGroups uses for every protected router group.
func (r *RouterGroup) Init(rg *gin.RouterGroup, _ *gorm.DB) {
	registerRoutes(r, rg)
}

func (r *RouterGroup) Register(rg *httpserver.Group, _ *gorm.DB) {
	rg.RegisterContract(http.MethodGet, "/me/bot-identities", httpserver.ResponseContract(
		response.Envelope[[]handlermeidentities.BotIdentityDTO]{}, http.StatusOK,
		http.StatusUnauthorized, http.StatusInternalServerError,
	))
	unlinkContract := httpserver.ResponseContract(
		response.Envelope[handlermeidentities.EntryIdentityUnlinkResult]{}, http.StatusAccepted, http.StatusNoContent,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	).WithParameters(httpserver.PathString("botId"), httpserver.RequiredQueryString("operation_id"))
	rg.RegisterContract(http.MethodDelete, "/me/bot-identities/:botId", unlinkContract)
	rg.RegisterContract(http.MethodGet, "/me/bot-identities/:botId/unlink-status", httpserver.ResponseContract(
		response.Envelope[handlermeidentities.EntryIdentityUnlinkResult]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError,
	).WithParameters(httpserver.PathString("botId"), httpserver.RequiredQueryString("operation_id")))
	rg.RegisterContract(http.MethodPost, "/me/entry-identity-links/preview", httpserver.JSONContract(
		handlermeidentities.EntryIdentityChallengeRequest{},
		response.Envelope[handlermeidentities.EntryIdentityChallengeView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusGone, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/me/entry-identity-links/approve", httpserver.JSONContract(
		handlermeidentities.EntryIdentityChallengeRequest{},
		response.Envelope[handlermeidentities.BotIdentityDTO]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusGone, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/me/entry-identity-links/cancel", httpserver.JSONContract(
		handlermeidentities.EntryIdentityChallengeRequest{}, nil, http.StatusNoContent,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusGone, http.StatusConflict, http.StatusInternalServerError,
	))
	registerRoutes(r, rg)
}

func registerRoutes[T httpserver.RouteGroup[T]](_ *RouterGroup, rg T) {
	h := handler.ApiGroupApp.MeIdentitiesApiGroup.Identities
	identities := rg.Group("/me/bot-identities")
	{
		identities.GET("", h.List)
		identities.DELETE("/:botId", h.Unlink)
		identities.GET("/:botId/unlink-status", h.UnlinkStatus)
	}
	links := rg.Group("/me/entry-identity-links")
	{
		links.POST("/preview", h.PreviewLink)
		links.POST("/approve", h.ApproveLink)
		links.POST("/cancel", h.CancelLink)
	}
}
