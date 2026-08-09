package meidentities

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"paigram/internal/middleware"
	"paigram/internal/model"
	"paigram/internal/response"
	"paigram/internal/service/botlink"
)

// BotIdentityDTO is the wire shape returned by GET /me/bot-identities.
// Field names and JSON tags follow spec §5.4.
type BotIdentityDTO struct {
	BotID            string  `json:"bot_id"`
	ExternalUserID   string  `json:"external_user_id"`
	ExternalUsername *string `json:"external_username,omitempty"`
	LinkedAt         string  `json:"linked_at"` // RFC3339
}

// Handler serves the /me/bot-identities surface.
//
// All endpoints require an authenticated session (user_id resolved via
// middleware.GetUserID); routes must be mounted behind AuthMiddleware +
// SessionValidation in the router layer.
type Handler struct {
	svc    *botlink.Service
	logger *zap.Logger
}

// NewHandler constructs a Handler with the supplied service + logger.
// A nil logger is replaced with zap.NewNop() to keep the handler safe to
// use in tests and during partial wiring.
func NewHandler(svc *botlink.Service, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{svc: svc, logger: logger}
}

// swagger:route GET /api/v1/me/bot-identities meidentities listMeBotIdentities
//
// List bot identities for the current user.
//
// Returns every active bot_identities row owned by the authenticated user,
// ordered by linked_at DESC (most recent first). Soft-deleted rows are
// excluded by the service's default scope. The response body is wrapped in
// the project-wide response envelope (account-center/AGENTS.md §4):
// {"code":200, "data":[...], "message":"success"}.
//
// Produces:
//   - application/json
//
// Security:
//   - BearerAuth: []
//
// Responses:
//
//	200: swaggerListResponse
//	401: swaggerErrorResponse
//	500: swaggerErrorResponse
//
// List handles GET /api/v1/me/bot-identities.
func (h *Handler) List(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == 0 {
		h.logger.Warn("unauthorized request to meidentities",
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
		)
		response.Unauthorized(c, "unauthorized")
		return
	}

	rows, err := h.svc.ListForUser(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("meidentities: list failed",
			zap.Uint64("user_id", userID),
			zap.Error(err),
		)
		response.InternalServerError(c, "internal")
		return
	}

	// Always use make([]…, 0, n) so the JSON encoder emits "[]" for the
	// empty case rather than "null". The frontend (Vue page bot-identities.vue
	// reading response.data ?? []) tolerates null, but [] matches the spec
	// §5.4 description and avoids client-side null-handling surprises.
	dtos := make([]BotIdentityDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, toDTO(r))
	}
	response.Success(c, dtos)
}

// swagger:route DELETE /api/v1/me/bot-identities/{botId} meidentities unlinkMeBotIdentity
//
// Unlink a bot identity from the current user.
//
// Deletes the (user_id, bot_id) row owned by the authenticated user. The
// response is intentionally opaque (spec §7.3): both "row does not exist"
// and "row exists but belongs to another user" return 404 with a
// byte-identical body so that an attacker cannot probe other users' link
// state via this endpoint.
//
// On success an audit log row (action=telegram_link_revoked) is written by
// the botlink service inside the same transaction as the soft-delete.
//
// Produces:
//   - application/json
//
// Security:
//   - BearerAuth: []
//
// Responses:
//
//	204: description: bot identity unlinked
//	401: swaggerErrorResponse
//	404: swaggerErrorResponse
//	500: swaggerErrorResponse
//
// Unlink handles DELETE /api/v1/me/bot-identities/:botId.
func (h *Handler) Unlink(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == 0 {
		h.logger.Warn("unauthorized request to meidentities",
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
		)
		response.Unauthorized(c, "unauthorized")
		return
	}

	botID := c.Param("botId")
	if botID == "" {
		// Defensive — gin should not route an empty :botId in normal
		// operation, but if it does we still keep the response opaque.
		response.NotFound(c, "not_found")
		return
	}

	err := h.svc.Unlink(
		c.Request.Context(),
		userID,
		botID,
		c.ClientIP(),
		c.Request.UserAgent(),
	)
	switch {
	case err == nil:
		response.NoContent(c)
	case errors.Is(err, botlink.ErrBotIdentityNotFound):
		// Spec §7.3: opaque 404 for both "no such row" and "row belongs
		// to another user". The service's WHERE user_id = ? AND bot_id = ?
		// filter naturally returns ErrBotIdentityNotFound for both cases,
		// so we never need to distinguish them here. 403 would leak
		// existence of the row to the attacker.
		response.NotFound(c, "not_found")
	default:
		h.logger.Error("meidentities: unlink failed",
			zap.Uint64("user_id", userID),
			zap.String("bot_id", botID),
			zap.Error(err),
		)
		response.InternalServerError(c, "internal")
	}
}

// toDTO projects a model.BotIdentity onto the wire shape, formatting
// LinkedAt as RFC3339 and unwrapping the sql.NullString external_username
// into an omitempty *string per spec §5.4.
func toDTO(r model.BotIdentity) BotIdentityDTO {
	dto := BotIdentityDTO{
		BotID:          r.BotID,
		ExternalUserID: r.ExternalUserID,
		LinkedAt:       r.LinkedAt.Format(time.RFC3339),
	}
	if r.ExternalUsername.Valid {
		v := r.ExternalUsername.String
		dto.ExternalUsername = &v
	}
	return dto
}
