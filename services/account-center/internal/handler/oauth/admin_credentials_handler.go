package oauth

import (
	"errors"

	"github.com/gin-gonic/gin"

	"paigram/internal/middleware"
	"paigram/internal/response"
	"paigram/internal/service/credentials"
)

// credentialsService is the subset of credentials.Service that the
// CredentialsHandler depends on; declared as an interface to keep the
// handler unit-testable.
type credentialsService interface {
	List() ([]credentials.CredentialView, error)
	Create(credentials.CreateInput) (*credentials.CreateResult, error)
	RotateSecret(clientID string) (*credentials.CreateResult, error)
}

// CredentialsHandler exposes admin CRUD for OAuth client credentials.
// All routes mount under /admin/service-credentials and are gated by the
// admin-role + casbin middleware in router/oauth/enter.go.
type CredentialsHandler struct {
	credentials credentialsService
}

// NewCredentialsHandler wires the admin handler against a credentials.Service.
func NewCredentialsHandler(credentialsSvc credentialsService) *CredentialsHandler {
	return &CredentialsHandler{credentials: credentialsSvc}
}

// CreateRequest is the JSON body for POST /admin/service-credentials.
type CreateRequest struct {
	ClientID    string   `json:"client_id"`
	BotID       string   `json:"bot_id"`
	EntryIssuer string   `json:"entry_issuer"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Audiences   []string `json:"audiences"`
	Scopes      []string `json:"scopes"`
}

// List returns all credentials in deterministic order. The plaintext
// secret is never returned here (and is hidden from the JSON projection
// of the underlying model via the `json:"-"` tag on SecretHash).
//
// @Tags        AdminCredentials
// @Summary     List OAuth service credentials
// @Description Returns every registered service credential (active and
// @Description disabled, soft-deletes excluded) in client_id ASC order.
// @Description Plaintext secrets are NEVER returned; only the bcrypt hash
// @Description metadata is exposed via the SecretHash json:"-" suppression.
// @Security    BearerAuth
// @Produce     json
// @Success     200 {object} response.Response{data=[]credentials.CredentialView}
// @Failure     401 {object} response.Response
// @Failure     403 {object} response.Response
// @Failure     500 {object} response.Response
// @Router      /api/v1/admin/service-credentials [get]
func (h *CredentialsHandler) List(c *gin.Context) {
	views, err := h.credentials.List()
	if err != nil {
		response.InternalServerError(c, "failed to list service credentials")
		return
	}
	response.Success(c, views)
}

// Create registers a new client_id + freshly-generated client_secret pair.
// The plaintext client_secret is returned exactly once in the response;
// the operator MUST capture it. Subsequent reads expose only metadata.
//
// @Tags        AdminCredentials
// @Summary     Create OAuth service credential
// @Description Registers a new client_id (operator-supplied) and generates
// @Description a fresh plaintext client_secret. The plaintext is returned
// @Description in the response body exactly once — there is no way to
// @Description recover it afterwards; operators MUST capture it at create
// @Description time. Subsequent reads expose only metadata.
// @Security    BearerAuth
// @Accept      json
// @Produce     json
// @Param       data body     CreateRequest                                                     true "Credential definition"
// @Success     201  {object} response.Response{data=credentials.CreateResult}                       "Includes one-time plaintext client_secret"
// @Failure     400  {object} response.Response
// @Failure     401  {object} response.Response
// @Failure     403  {object} response.Response
// @Failure     500  {object} response.Response
// @Router      /api/v1/admin/service-credentials [post]
func (h *CredentialsHandler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	actorID, _ := middleware.GetUserID(c)
	result, err := h.credentials.Create(credentials.CreateInput{
		ClientID:    req.ClientID,
		BotID:       req.BotID,
		EntryIssuer: req.EntryIssuer,
		DisplayName: req.DisplayName,
		OwnerUserID: actorID,
		Description: req.Description,
		Audiences:   req.Audiences,
		Scopes:      req.Scopes,
	})
	if err != nil {
		writeAdminError(c, err, "failed to create service credential")
		return
	}
	response.Created(c, result)
}

// RotateSecret regenerates the bcrypt secret for an existing credential.
// The old secret stops working immediately; there is no grace period.
//
// @Tags        AdminCredentials
// @Summary     Rotate OAuth service credential secret
// @Description Regenerates the plaintext client_secret for an existing
// @Description client_id and replaces the stored bcrypt hash atomically.
// @Description The previous secret is invalidated immediately — there is
// @Description no grace period.
// @Description The new plaintext is returned exactly once in the response.
// @Security    BearerAuth
// @Produce     json
// @Param       client_id path     string                                            true "Registered service credential id"
// @Success     200       {object} response.Response{data=credentials.CreateResult}       "Includes new one-time plaintext client_secret"
// @Failure     400       {object} response.Response
// @Failure     401       {object} response.Response
// @Failure     403       {object} response.Response
// @Failure     404       {object} response.Response
// @Failure     500       {object} response.Response
// @Router      /api/v1/admin/service-credentials/{client_id}/secret [post]
func (h *CredentialsHandler) RotateSecret(c *gin.Context) {
	clientID := c.Param("client_id")
	if clientID == "" {
		response.BadRequest(c, "client_id is required")
		return
	}
	result, err := h.credentials.RotateSecret(clientID)
	if err != nil {
		writeAdminError(c, err, "failed to rotate service credential secret")
		return
	}
	response.Success(c, result)
}

func writeAdminError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, credentials.ErrEmptyClientID):
		response.BadRequest(c, "client_id is required")
	case errors.Is(err, credentials.ErrCredentialNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, credentials.ErrCredentialConflict):
		response.Conflict(c, err.Error())
	case errors.Is(err, credentials.ErrBotIssuerConflict):
		response.Conflict(c, err.Error())
	case errors.Is(err, credentials.ErrInvalidEntryIssuer):
		response.BadRequest(c, err.Error())
	default:
		response.InternalServerError(c, fallback)
	}
}
