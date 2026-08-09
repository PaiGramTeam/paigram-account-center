package adminsystem

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"paigram/internal/response"
	"paigram/internal/service/botroute"
)

// botRouteAdmin is the narrow business interface consumed by the admin REST
// handler. Defining it on the consumer side lets tests substitute fakes
// without depending on the concrete *botroute.Service.
type botRouteAdmin interface {
	ListBotRoutes(ctx context.Context) ([]botroute.BotRouteAdminView, error)
	GetBotRouteByID(ctx context.Context, id uint64) (*botroute.BotRouteAdminView, error)
	DeleteBotRoute(ctx context.Context, id uint64) error
}

// BotRouteHandler serves the admin system bot route ops endpoints. Read +
// hard-delete only; registration must happen via the gRPC contract used by
// game-services.
type BotRouteHandler struct {
	service botRouteAdmin
}

// NewBotRouteHandler builds a bot route admin handler.
func NewBotRouteHandler(service botRouteAdmin) *BotRouteHandler {
	return &BotRouteHandler{service: service}
}

// ListBotRoutes lists all registered bot routes.
// swagger:route GET /api/v1/admin/system/bot-routes admin-system listBotRoutes
//
// List bot routes.
//
//	Responses:
//	  200: description: List of bot routes
func (h *BotRouteHandler) ListBotRoutes(c *gin.Context) {
	items, err := h.service.ListBotRoutes(c.Request.Context())
	if err != nil {
		writeBotRouteError(c, err, "failed to list bot routes")
		return
	}
	response.Success(c, items)
}

// GetBotRoute returns one bot route by primary key.
// swagger:route GET /api/v1/admin/system/bot-routes/{id} admin-system getBotRoute
//
// Get a bot route.
//
//	Responses:
//	  200: description: The bot route
//	  404: description: Not found
func (h *BotRouteHandler) GetBotRoute(c *gin.Context) {
	id, ok := parseBotRouteID(c)
	if !ok {
		return
	}
	item, err := h.service.GetBotRouteByID(c.Request.Context(), id)
	if err != nil {
		writeBotRouteError(c, err, "failed to get bot route")
		return
	}
	response.Success(c, item)
}

// DeleteBotRoute hard-deletes a bot route by primary key.
// swagger:route DELETE /api/v1/admin/system/bot-routes/{id} admin-system deleteBotRoute
//
// Delete a bot route.
//
//	Responses:
//	  204: description: Deleted
//	  404: description: Not found
func (h *BotRouteHandler) DeleteBotRoute(c *gin.Context) {
	id, ok := parseBotRouteID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteBotRoute(c.Request.Context(), id); err != nil {
		writeBotRouteError(c, err, "failed to delete bot route")
		return
	}
	c.Status(http.StatusNoContent)
}

func parseBotRouteID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid bot route id")
		return 0, false
	}
	return id, true
}

func writeBotRouteError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, botroute.ErrRouteNotFound):
		response.NotFound(c, "bot route not found")
	case errors.Is(err, botroute.ErrInvalidRouteRequest):
		response.BadRequest(c, err.Error())
	default:
		response.InternalServerError(c, fallback)
	}
}
