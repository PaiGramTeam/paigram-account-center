package meidentities

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"paigram/internal/middleware"
	"paigram/internal/model"
	"paigram/internal/response"
	"paigram/internal/service/botlink"
	"paigram/internal/service/entryidentity"
)

// BotIdentityDTO is the wire shape returned by GET /me/bot-identities.
type BotIdentityDTO struct {
	BotID            string  `json:"bot_id"`
	Issuer           string  `json:"issuer"`
	ExternalUserID   string  `json:"external_user_id"`
	ExternalUsername *string `json:"external_username,omitempty"`
	LinkedAt         string  `json:"linked_at"` // RFC3339
}

type EntryIdentityChallengeRequest struct {
	Challenge string `json:"challenge" binding:"required"`
}

type EntryIdentityChallengeView struct {
	Issuer         string    `json:"issuer"`
	MaskedSubject  string    `json:"masked_subject"`
	BotID          string    `json:"bot_id"`
	BotDisplayName string    `json:"bot_display_name"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type EntryIdentityUnlinkResult = entryidentity.UnlinkResult

// Handler serves the /me/bot-identities surface.
//
// All endpoints require an authenticated session (user_id resolved via
// middleware.GetUserID); routes must be mounted behind AuthMiddleware +
// SessionValidation in the router layer.
type Handler struct {
	svc     *botlink.Service
	linking *entryidentity.Service
	logger  *zap.Logger
}

// NewHandler constructs a Handler with the supplied service + logger.
// A nil logger is replaced with zap.NewNop() to keep the handler safe to
// use in tests and during partial wiring.
func NewHandler(svc *botlink.Service, logger *zap.Logger, linking ...*entryidentity.Service) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	handler := &Handler{svc: svc, logger: logger}
	if len(linking) > 0 {
		handler.linking = linking[0]
	}
	return handler
}

func (h *Handler) PreviewLink(c *gin.Context) {
	markChallengeResponseNoStore(c)
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == 0 {
		response.Unauthorized(c, "unauthorized")
		return
	}
	request, ok := bindChallengeRequest(c)
	if !ok {
		return
	}
	if h.linking == nil {
		response.InternalServerError(c, "entry identity linking is unavailable")
		return
	}
	preview, err := h.linking.Preview(c.Request.Context(), request.Challenge)
	if err != nil {
		writeChallengeError(c, err)
		return
	}
	response.Success(c, challengeView(preview))
}

func (h *Handler) ApproveLink(c *gin.Context) {
	markChallengeResponseNoStore(c)
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == 0 {
		response.Unauthorized(c, "unauthorized")
		return
	}
	request, ok := bindChallengeRequest(c)
	if !ok {
		return
	}
	if h.linking == nil {
		response.InternalServerError(c, "entry identity linking is unavailable")
		return
	}
	identity, err := h.linking.Approve(c.Request.Context(), userID, request.Challenge, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		writeChallengeError(c, err)
		return
	}
	response.Success(c, toDTO(*identity))
}

func (h *Handler) CancelLink(c *gin.Context) {
	markChallengeResponseNoStore(c)
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == 0 {
		response.Unauthorized(c, "unauthorized")
		return
	}
	request, ok := bindChallengeRequest(c)
	if !ok {
		return
	}
	if h.linking == nil {
		response.InternalServerError(c, "entry identity linking is unavailable")
		return
	}
	if err := h.linking.Cancel(c.Request.Context(), userID, request.Challenge); err != nil {
		writeChallengeError(c, err)
		return
	}
	response.NoContent(c)
}

func markChallengeResponseNoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
}

func bindChallengeRequest(c *gin.Context) (EntryIdentityChallengeRequest, bool) {
	var request EntryIdentityChallengeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "challenge is required")
		return EntryIdentityChallengeRequest{}, false
	}
	return request, true
}

func writeChallengeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, entryidentity.ErrChallengeNotFound):
		response.NotFoundWithCode(c, "entry_identity_link_not_found", "entry identity link not found", nil)
	case errors.Is(err, entryidentity.ErrChallengeExpired):
		response.ErrorWithCode(c, http.StatusGone, "entry_identity_link_expired", "entry identity link expired", nil)
	case errors.Is(err, entryidentity.ErrChallengeConsumed):
		response.ConflictWithCode(c, "entry_identity_link_consumed", "entry identity link already used", nil)
	case errors.Is(err, entryidentity.ErrIdentityConflict):
		response.ConflictWithCode(c, "entry_identity_link_conflict", "entry identity is already linked", nil)
	case errors.Is(err, entryidentity.ErrUnlinkPending):
		response.ConflictWithCode(c, "entry_identity_unlink_pending", "entry identity unlink is still pending", nil)
	case errors.Is(err, entryidentity.ErrInvalidInput):
		response.BadRequestWithCode(c, "entry_identity_link_invalid", "invalid entry identity link", nil)
	case errors.Is(err, entryidentity.ErrPrincipalMismatch), errors.Is(err, entryidentity.ErrNamespaceUnavailable):
		response.ErrorWithCode(c, http.StatusGone, "entry_identity_namespace_unavailable", "entry identity namespace is no longer available", nil)
	default:
		response.InternalServerError(c, "entry identity link failed")
	}
}

func (h *Handler) UnlinkStatus(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == 0 {
		response.Unauthorized(c, "unauthorized")
		return
	}
	if h.linking == nil {
		response.InternalServerError(c, "entry identity linking is unavailable")
		return
	}
	result, err := h.linking.UnlinkStatus(c.Request.Context(), userID, c.Param("botId"), c.Query("operation_id"))
	switch {
	case err == nil:
		response.Success(c, result)
	case errors.Is(err, botlink.ErrBotIdentityNotFound):
		response.NotFoundWithCode(c, "entry_identity_unlink_operation_not_found", "entry identity unlink operation not found", nil)
	case errors.Is(err, entryidentity.ErrInvalidInput):
		response.BadRequest(c, "operation_id is required")
	default:
		h.logger.Error("meidentities: unlink status failed", zap.Error(err))
		response.InternalServerError(c, "internal")
	}
}

func challengeView(view *entryidentity.ChallengeView) EntryIdentityChallengeView {
	return EntryIdentityChallengeView{
		Issuer: view.Issuer, MaskedSubject: view.MaskedSubject, BotID: view.BotID,
		BotDisplayName: view.BotDisplayName, ExpiresAt: view.ExpiresAt,
	}
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

	// Use a non-nil slice so the wire response is [] instead of null.
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
// response is intentionally opaque: both "row does not exist"
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
//	202: description: local unlink accepted and fence propagation pending
//	204: description: bot identity unlinked and fence confirmed
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
	operationID := c.Query("operation_id")
	if operationID == "" {
		response.BadRequest(c, "operation_id is required")
		return
	}

	var err error
	var unlinkResult *entryidentity.UnlinkResult
	if h.linking != nil {
		unlinkResult, err = h.linking.Unlink(c.Request.Context(), userID, botID, operationID, c.ClientIP(), c.Request.UserAgent())
	} else {
		err = h.svc.Unlink(c.Request.Context(), userID, botID, c.ClientIP(), c.Request.UserAgent())
	}
	switch {
	case err == nil && unlinkResult != nil && unlinkResult.PropagationPending:
		response.Custom(c, http.StatusAccepted, http.StatusAccepted, unlinkResult, "revocation propagation pending")
	case err == nil:
		response.NoContent(c)
	case errors.Is(err, botlink.ErrBotIdentityNotFound):
		// Keep the response opaque for both "no such row" and "row belongs
		// to another user". The service's WHERE user_id = ? AND bot_id = ?
		// filter naturally returns ErrBotIdentityNotFound for both cases,
		// so we never need to distinguish them here. 403 would leak
		// existence of the row to the attacker.
		response.NotFoundWithCode(c, "entry_identity_unlink_operation_not_found", "entry identity unlink operation not found", nil)
	case errors.Is(err, entryidentity.ErrInvalidInput):
		response.BadRequest(c, "operation_id must be a canonical UUID")
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
// LinkedAt as RFC3339 and unwrapping the optional external username.
func toDTO(r model.BotIdentity) BotIdentityDTO {
	dto := BotIdentityDTO{
		BotID:          r.BotID,
		Issuer:         r.Issuer,
		ExternalUserID: r.ExternalUserID,
		LinkedAt:       r.LinkedAt.Format(time.RFC3339),
	}
	if r.ExternalUsername.Valid {
		v := r.ExternalUsername.String
		dto.ExternalUsername = &v
	}
	return dto
}
