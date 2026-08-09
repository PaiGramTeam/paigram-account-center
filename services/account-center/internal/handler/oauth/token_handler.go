package oauth

import (
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"paigram/internal/service/credentials"
)

// rfc6749ErrorResponse is the RFC 6749 §5.2 token-error-response shape.
// Content-Type stays application/json per the same section.
type rfc6749ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// rfc6749 error codes (§5.2) we emit.
const (
	errInvalidRequest       = "invalid_request"
	errInvalidClient        = "invalid_client"
	errInvalidScope         = "invalid_scope"
	errUnsupportedGrantType = "unsupported_grant_type"
)

const grantTypeClientCredentials = "client_credentials"

// tokenServiceFacade abstracts the credentials.TokenService method we
// consume; declaring an interface here lets tests inject a fake.
type tokenServiceFacade interface {
	IssueClientCredentials(credentials.IssueClientCredentialsInput) (*credentials.IssuedToken, error)
}

// TokenHandler serves POST /oauth/token (RFC 6749 §4.4 client_credentials).
type TokenHandler struct {
	tokens tokenServiceFacade
}

// NewTokenHandler wires the handler against a credentials.TokenService.
func NewTokenHandler(tokens tokenServiceFacade) *TokenHandler {
	return &TokenHandler{tokens: tokens}
}

// Token issues a Bearer access_token under RFC 6749 §4.4 client_credentials.
//
// Request body MUST be application/x-www-form-urlencoded per §4.4.2.
// JSON bodies and any other Content-Type are rejected with `invalid_request`.
//
// Required form fields: grant_type=client_credentials, client_id,
// client_secret, audience. Optional: scope (space-delimited; defaults to
// the credential's granted scope set).
//
// Success: 200 OK, body per §5.1: {access_token, token_type, expires_in,
// scope}. Failure: 400/401 with the §5.2 {error, error_description} shape.
//
// @Tags        OAuth
// @Summary     Issue OAuth 2.0 client_credentials access token
// @Description RFC 6749 §4.4 client_credentials grant. Authenticates via the
// @Description (client_id, client_secret) pair carried in the form body; no
// @Description Authorization header is required or honoured.
// @Accept      x-www-form-urlencoded
// @Produce     json
// @Param       grant_type    formData  string  true   "Must be 'client_credentials'"
// @Param       client_id     formData  string  true   "Registered service credential id"
// @Param       client_secret formData  string  true   "Plaintext client secret"
// @Param       audience      formData  string  true   "Target service audience (must be in the credential's allow-list)"
// @Param       scope         formData  string  false  "Optional space-delimited scope subset (RFC 6749 §3.3)"
// @Success     200 {object}  credentials.IssuedToken         "Access token response (§5.1)"
// @Failure     400 {object}  rfc6749ErrorResponse            "invalid_request / unsupported_grant_type / invalid_scope"
// @Failure     401 {object}  rfc6749ErrorResponse            "invalid_client"
// @Failure     500 {object}  rfc6749ErrorResponse            "server_error"
// @Router      /api/v1/oauth/token [post]
func (h *TokenHandler) Token(c *gin.Context) {
	// RFC 6749 §4.4.2 mandates application/x-www-form-urlencoded. Use
	// mime.ParseMediaType so optional parameters (e.g. "charset=utf-8")
	// and surrounding whitespace canonicalise correctly instead of
	// being matched naively against the raw header string.
	mediaType, _, parseErr := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if parseErr != nil || !strings.EqualFold(mediaType, "application/x-www-form-urlencoded") {
		writeOAuthError(c, http.StatusBadRequest, errInvalidRequest, "Content-Type must be application/x-www-form-urlencoded")
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		writeOAuthError(c, http.StatusBadRequest, errInvalidRequest, "malformed form body")
		return
	}

	grantType := c.Request.PostFormValue("grant_type")
	if grantType != grantTypeClientCredentials {
		writeOAuthError(c, http.StatusBadRequest, errUnsupportedGrantType, "only client_credentials is supported")
		return
	}
	clientID := c.Request.PostFormValue("client_id")
	clientSecret := c.Request.PostFormValue("client_secret")
	audience := c.Request.PostFormValue("audience")
	if clientID == "" || clientSecret == "" {
		writeOAuthError(c, http.StatusBadRequest, errInvalidRequest, "client_id and client_secret are required")
		return
	}
	if audience == "" {
		writeOAuthError(c, http.StatusBadRequest, errInvalidRequest, "audience is required")
		return
	}

	scopes := credentials.SplitScope(c.Request.PostFormValue("scope"))

	issued, err := h.tokens.IssueClientCredentials(credentials.IssueClientCredentialsInput{
		ClientID:        clientID,
		ClientSecret:    clientSecret,
		Audience:        audience,
		RequestedScopes: scopes,
	})
	if err != nil {
		writeTokenError(c, err)
		return
	}

	// RFC 6749 §5.1 mandates no-store cache headers on token responses.
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, issued)
}

func writeTokenError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, credentials.ErrCredentialNotFound),
		errors.Is(err, credentials.ErrCredentialDisabled),
		errors.Is(err, credentials.ErrInvalidClientSecret),
		errors.Is(err, credentials.ErrEmptyClientID):
		writeOAuthError(c, http.StatusUnauthorized, errInvalidClient, "client authentication failed")
	case errors.Is(err, credentials.ErrInvalidAudience):
		writeOAuthError(c, http.StatusBadRequest, errInvalidRequest, "audience not permitted for this credential")
	case errors.Is(err, credentials.ErrInsufficientScope):
		writeOAuthError(c, http.StatusBadRequest, errInvalidScope, "requested scope exceeds granted scope")
	default:
		writeOAuthError(c, http.StatusInternalServerError, "server_error", "failed to issue token")
	}
}

func writeOAuthError(c *gin.Context, statusCode int, code, description string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.AbortWithStatusJSON(statusCode, rfc6749ErrorResponse{Error: code, ErrorDescription: description})
}
