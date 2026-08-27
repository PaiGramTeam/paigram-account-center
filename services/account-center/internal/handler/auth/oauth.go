package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"paigram/internal/config"
	"paigram/internal/logging"
	"paigram/internal/middleware"
	"paigram/internal/model"
	"paigram/internal/response"
	"paigram/internal/service"
	"paigram/internal/utils/secsubtle"
)

var (
	errProviderAlreadyBound   = errors.New("provider already bound to another user")
	errProviderRebindConflict = errors.New("provider already bound to a different account on this user")
	errMissingBindUser        = errors.New("missing oauth bind user")
	errBindAuthRequired       = errors.New("bind callback requires authenticated user")
	errBindUserMismatch       = errors.New("bind callback authenticated user mismatch")
)

// swagger:route POST /api/v1/auth/oauth/{provider}/init auth initiateOAuth
//
// Initiate OAuth authentication flow.
//
// Generates OAuth state and nonce tokens, then returns the provider's
// authorization URL for the user to complete authentication.
//
// Consumes:
//   - application/json
//
// Produces:
//   - application/json
//
// Responses:
//
//	200: initiateOAuthResponse
//	400: authErrorResponse
//	500: authErrorResponse
//
// InitiateOAuth prepares an OAuth login by issuing a state token.
func (h *Handler) InitiateOAuth(c *gin.Context) {
	h.initiateOAuth(c, model.OAuthPurposeLogin, nil)
}

// swagger:route PUT /api/v1/me/login-methods/{provider} me startBindLoginMethod
//
// Initiate OAuth login-method binding for the authenticated user.
//
// Requires an authenticated fresh session. Generates OAuth state bound to the
// current user and returns the provider authorization URL.
//
// Consumes:
//   - application/json
//
// Produces:
//   - application/json
//
// Responses:
//
//	200: initiateOAuthResponse
//	401: authErrorResponse
//	403: authErrorResponse
//	400: authErrorResponse
//	500: authErrorResponse
func (h *Handler) StartBindLoginMethod(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == 0 {
		response.UnauthorizedWithCode(c, "UNAUTHORIZED", "user not authenticated", nil)
		return
	}
	h.initiateOAuth(c, model.OAuthPurposeBindLoginMethod, &userID)
}

func (h *Handler) initiateOAuth(c *gin.Context, purpose model.OAuthPurpose, userID *uint64) {
	provider := strings.ToLower(c.Param("provider"))
	providerCfg, ok := h.resolveProvider(provider)
	if !ok {
		response.BadRequest(c, "unsupported provider")
		return
	}

	var req InitiateOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			logging.Error("initiate oauth: invalid request body", zap.Error(err), zap.String("provider", provider))
			response.BadRequest(c, "invalid request body")
			return
		}
	}

	state, err := randomToken(32)
	if err != nil {
		response.InternalServerError(c, "failed to generate state")
		return
	}
	nonce, err := randomToken(24)
	if err != nil {
		response.InternalServerError(c, "failed to generate nonce")
		return
	}

	// Generate PKCE code verifier and challenge
	codeVerifier, codeChallenge, err := generatePKCE()
	if err != nil {
		response.InternalServerError(c, "failed to generate PKCE")
		return
	}

	redirectURL, err := resolveOAuthRedirectURI(req.RedirectTo, providerCfg, h.cfg.DefaultOAuthRedirectURL)
	if err != nil {
		logging.Warn("oauth redirect URI rejected", zap.Error(err), zap.String("provider", provider))
		response.BadRequestWithCode(c, "OAUTH_REDIRECT_URI_INVALID", "OAuth redirect URI is not allowed", nil)
		return
	}

	stateTTL := time.Duration(h.cfg.OAuthStateTTLSeconds) * time.Second
	if stateTTL <= 0 {
		stateTTL = 5 * time.Minute
	}
	expiry := time.Now().UTC().Add(stateTTL)

	stateRecord := model.UserOAuthState{
		Provider:     provider,
		State:        state,
		Purpose:      string(purpose),
		RedirectTo:   redirectURL,
		Nonce:        nonce,
		CodeVerifier: codeVerifier, // Store for later verification
		ClientIP:     c.ClientIP(),
		UserAgent:    truncateUserAgent(c.GetHeader("User-Agent")),
		ExpiresAt:    expiry,
	}
	if userID != nil {
		stateRecord.UserID = sql.NullInt64{Int64: int64(*userID), Valid: true}
	}
	if err := h.db.Create(&stateRecord).Error; err != nil {
		response.InternalServerError(c, "failed to persist oauth state")
		return
	}

	authURL, err := buildAuthURL(providerCfg, redirectURL, state, nonce, codeChallenge)
	if err != nil {
		response.InternalServerError(c, "failed to build auth url")
		return
	}

	responseData := map[string]interface{}{
		"auth_url":   authURL,
		"state":      state,
		"expires_at": expiry.Format(time.RFC3339),
		"purpose":    string(purpose),
	}
	response.Success(c, responseData)
}

// swagger:route POST /api/v1/auth/oauth/{provider}/callback auth handleOAuthCallback
//
// Handle OAuth provider callback.
//
// Processes the OAuth callback after user authorization at the provider.
// Creates or updates the user account and returns JWT tokens.
//
// Consumes:
//   - application/json
//
// Produces:
//   - application/json
//
// Security:
//   - none
//
// Responses:
//
//	200: oauthCallbackResponse
//	400: authErrorResponse
//	401: authErrorResponse
//	403: authErrorResponse
//	409: authErrorResponse
//	500: authErrorResponse
//
// HandleOAuthCallback processes the OAuth callback and issues a local session.
// Login-purpose callbacks return a login session payload; bind-purpose callbacks return
// a bind result payload for the authenticated user. Bind-purpose callbacks require the
// current authenticated user to match the persisted state owner.
func (h *Handler) HandleOAuthCallback(c *gin.Context) {
	provider := strings.ToLower(c.Param("provider"))
	providerCfg, ok := h.resolveProvider(provider)
	if !ok {
		response.BadRequest(c, "unsupported provider")
		return
	}

	var req OAuthCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logging.Error("oauth callback: invalid request body", zap.Error(err), zap.String("provider", provider))
		response.BadRequest(c, "invalid request body")
		return
	}

	now := time.Now().UTC()
	clientIP := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	// Step 1: Atomically consume the OAuth state. This single transaction
	// covers SELECT (FOR UPDATE), expiry check, IP/UA binding check, bind
	// pre-authorization, and DELETE — preventing the TOCTOU window the
	// previous separate-statement implementation allowed (V23).
	stateRecordPtr, err := h.consumeOAuthState(c, provider, req.State, now)
	if err != nil {
		switch {
		case errors.Is(err, errStateNotFound):
			response.BadRequest(c, "invalid oauth state")
		case errors.Is(err, errStateExpired):
			response.BadRequest(c, "oauth state expired")
		case errors.Is(err, errStateClientChanged):
			// Don't reveal which dimension (IP vs UA) mismatched — the
			// generic "invalid oauth state" matches what an unrelated state
			// would return, denying the attacker an oracle.
			response.BadRequest(c, "invalid oauth state")
		case errors.Is(err, errBindAuthRequired):
			response.UnauthorizedWithCode(c, "UNAUTHORIZED", "bind callback requires authentication", nil)
		case errors.Is(err, errBindUserMismatch):
			response.ForbiddenWithCode(c, "FORBIDDEN", "authenticated user does not match bind state", nil)
		case errors.Is(err, errMissingBindUser):
			response.BadRequest(c, "invalid oauth state")
		default:
			response.InternalServerError(c, "database error")
		}
		return
	}
	stateRecord := *stateRecordPtr

	purpose := oauthPurposeFromState(stateRecord)

	// Step 2: Exchange authorization code for tokens (BACKEND ONLY) with PKCE
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tokenResp, err := h.exchangeCodeForToken(
		ctx,
		provider,
		req.Code,
		stateRecord.CodeVerifier,
		stateRecord.RedirectTo,
		providerCfg,
	)
	if err != nil {
		logging.Warn("oauth token exchange failed", zap.Error(err), zap.String("provider", provider))
		response.BadRequestWithCode(c, "OAUTH_TOKEN_EXCHANGE_FAILED", "OAuth provider rejected the authorization response", nil)
		return
	}

	// Step 2.5: Verify ID token claims (OIDC)
	idTokenClaims, err := h.verifyIDToken(ctx, provider, tokenResp.IDToken, providerCfg, stateRecord.Nonce)
	if err != nil {
		logging.Warn("oauth ID token validation failed", zap.Error(err), zap.String("provider", provider))
		response.BadRequestWithCode(c, "OAUTH_ID_TOKEN_INVALID", "OAuth identity token is invalid", nil)
		return
	}

	// Step 2.6: Validate scopes
	scopeWarnings := validateScopes(providerCfg.Scopes, tokenResp.Scope)
	if len(scopeWarnings) > 0 {
		// Log warnings but don't fail - some providers may grant subset of scopes
		for _, warning := range scopeWarnings {
			log.Printf("[OAuth] Scope warning for provider %s: %s", provider, warning)
		}
	}

	// Step 3: Fetch user info from provider
	userInfo, err := h.fetchUserInfo(ctx, provider, tokenResp.AccessToken, providerCfg, idTokenClaims)
	if err != nil {
		logging.Warn("oauth userinfo request failed", zap.Error(err), zap.String("provider", provider))
		response.ErrorWithCode(c, http.StatusBadGateway, "OAUTH_USERINFO_FAILED", "OAuth provider profile is unavailable", nil)
		return
	}
	identity, err := resolveOAuthIdentity(provider, providerCfg, idTokenClaims, userInfo)
	if err != nil {
		logging.Error("oauth identity validation failed", zap.Error(err), zap.String("provider", provider))
		response.BadRequestWithCode(c, "OAUTH_IDENTITY_INVALID", "OAuth identity is invalid", nil)
		return
	}
	userInfo.Issuer = identity.Issuer
	userInfo.ID = identity.Subject

	if purpose == model.OAuthPurposeBindLoginMethod {
		err = h.bindOAuthLoginMethod(provider, stateRecord, userInfo, tokenResp, now)
		if err != nil {
			if errors.Is(err, errProviderAlreadyBound) {
				response.ConflictWithCode(c, "PROVIDER_ALREADY_BOUND", "provider account is already bound to another user", nil)
				return
			}
			if errors.Is(err, errProviderRebindConflict) {
				response.ConflictWithCode(c, "PROVIDER_REBIND_CONFLICT", "provider is already bound to a different account on this user", nil)
				return
			}
			if errors.Is(err, errMissingBindUser) {
				response.BadRequest(c, "invalid oauth state")
				return
			}
			logging.Error("oauth bind failed", zap.Error(err), zap.String("provider", provider))
			response.BadRequest(c, "oauth bind failed")
			return
		}

		response.Success(c, map[string]interface{}{
			"provider":            provider,
			"provider_account_id": userInfo.ID,
			"purpose":             string(purpose),
			"user_id":             uint64(stateRecord.UserID.Int64),
			"bound":               true,
		})
		return
	}

	result, err := h.completeOAuthLogin(provider, userInfo, tokenResp, now, clientIP, userAgent)

	if err != nil {
		logging.Error("oauth login completion failed", zap.Error(err), zap.String("provider", provider))
		response.BadRequest(c, "oauth login failed")
		return
	}

	h.setBrowserRefreshCookie(c, result.sessionWithTokens.RefreshToken, result.sessionWithTokens.Session.RefreshExpiry)
	responseData := map[string]interface{}{
		"user_id":        result.user.ID,
		"access_token":   result.sessionWithTokens.AccessToken,
		"access_expiry":  result.sessionWithTokens.Session.AccessExpiry.Format(time.RFC3339),
		"refresh_expiry": result.sessionWithTokens.Session.RefreshExpiry.Format(time.RFC3339),
		"email":          emailValue(result.emailRecord),
	}
	response.Success(c, responseData)
}

var (
	errStateNotFound      = errors.New("oauth state not found")
	errStateExpired       = errors.New("oauth state expired")
	errStateClientChanged = errors.New("oauth state client binding mismatch")
)

// consumeOAuthState atomically validates and deletes a one-time OAuth state
// row. The lookup, IP/UA binding check, expiry check, and DELETE all happen
// inside a single transaction with a row-level lock (FOR UPDATE), so two
// concurrent callbacks cannot both succeed for the same state value (V23).
//
// On success the returned record carries the original Nonce/CodeVerifier so
// the caller can finish the OAuth code exchange. The row is already gone
// from the DB by the time this function returns.
//
// Failure modes and their state-row side effects:
//
//   - errStateNotFound: row missing (already consumed, never existed). No-op.
//   - errStateExpired:  row is expired. We DELETE it (committed) so it can't
//     be retried.
//   - errStateClientChanged: IP and/or UA does not match the value captured
//     at state creation. We DELETE it (committed). Strict full-IP equality
//     is documented in the schema migration; mobile NAT can cause false
//     positives, which we accept for now.
//   - errBindAuthRequired / errBindUserMismatch: bind-purpose pre-check
//     failed; the row is PRESERVED so the legitimate user can retry the
//     callback after authenticating. (This matches the historical contract
//     documented by TestHandleOAuthCallbackDoesNotConsumeStateWhenBindCallbackIsUnauthorized.)
//
// Implementation notes: the SELECT below uses
// `Clauses(clause.Locking{Strength: "UPDATE"})` to inject `FOR UPDATE` —
// this is the GORM v2 idiom. The pre-fix v1 idiom
// `Set("gorm:query_option", "FOR UPDATE")` is silently ignored in v2 and
// emits a plain SELECT, which would defeat V23 atomicity entirely. A
// regression test (TestConsumeOAuthState_EmitsForUpdateInSelect) captures
// the rendered SQL and asserts the lock clause is present.
//
// Concurrent consume attempts therefore serialize at the SELECT FOR UPDATE.
// The success-path DELETE additionally checks RowsAffected == 1 so that if
// (despite the lock) two transactions ever both reached the DELETE, only
// one wins; the loser sees RowsAffected == 0 and returns errStateNotFound,
// matching the "row already consumed" semantics expected by the caller.
func (h *Handler) consumeOAuthState(c *gin.Context, provider, state string, now time.Time) (*model.UserOAuthState, error) {
	clientIP := c.ClientIP()
	userAgent := truncateUserAgent(c.GetHeader("User-Agent"))

	tx := h.db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var record model.UserOAuthState
	// Critical: clause.Locking{Strength: "UPDATE"} is the GORM v2 way to
	// emit `FOR UPDATE`. Do NOT regress to `Set("gorm:query_option", ...)`
	// — that is a v1 idiom and is a no-op in v2 (verified empirically by
	// TestConsumeOAuthState_EmitsForUpdateInSelect; the previous code path
	// emitted a plain SELECT with no lock).
	if err := tx.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("state = ? AND provider = ?", state, provider).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errStateNotFound
		}
		return nil, err
	}

	if now.After(record.ExpiresAt) {
		if delErr := tx.Delete(&record).Error; delErr != nil {
			return nil, delErr
		}
		if commitErr := tx.Commit().Error; commitErr != nil {
			return nil, commitErr
		}
		committed = true
		return nil, errStateExpired
	}

	// Bind-purpose pre-authorization MUST run before the strict IP/UA
	// client-binding check below.
	//
	// Rationale (regression-tested by
	// TestHandleOAuthCallbackBindAuthMissingTakesPrecedenceOverUserAgentMismatch
	// and integration-tested by
	// TestOAuthBindFlowRequiresAuthenticatedSessionForCallback):
	//
	//   - A bind-purpose callback hit with a missing/expired session is
	//     the normal "session lost during the OAuth roundtrip" recovery
	//     path. The user must see 401 + a preserved state row so they can
	//     re-authenticate and retry without re-running the provider flow.
	//   - The IP/UA strict-binding check is a defense-in-depth signal,
	//     not a substitute for authentication. Running it first turned a
	//     401-recoverable case into a 400 with the row deleted — hostile
	//     UX and contractually wrong.
	//   - For login-purpose states (no bind owner) we skip this branch
	//     entirely and fall through to the IP/UA gate, preserving V23's
	//     anti-replay coverage for unauthenticated login flows.
	//
	// authorizeBindCallback returns errBindAuthRequired / errBindUserMismatch
	// / errMissingBindUser without touching the row; the deferred rollback
	// preserves it for the user's retry.
	if oauthPurposeFromState(record) == model.OAuthPurposeBindLoginMethod {
		if authErr := h.authorizeBindCallback(c, record); authErr != nil {
			return nil, authErr
		}
	}

	// Strict client-binding check. Use constant-time equality (V18) for
	// values an attacker could plausibly manipulate via header injection.
	// Empty stored values are treated as mismatch — production state rows
	// always carry a non-empty IP, and accepting "" would let pre-fix rows
	// pass the check.
	//
	// For bind-purpose states this runs only after authorizeBindCallback
	// confirmed the caller is the bind owner; an IP/UA mismatch at that
	// point is a stronger signal of a hijacked session and we DO consume
	// the row.
	if !secsubtle.StringEqual(record.ClientIP, clientIP) || !secsubtle.StringEqual(record.UserAgent, userAgent) {
		if delErr := tx.Delete(&record).Error; delErr != nil {
			return nil, delErr
		}
		if commitErr := tx.Commit().Error; commitErr != nil {
			return nil, commitErr
		}
		committed = true
		return nil, errStateClientChanged
	}

	// All checks passed; consume the row inside the same tx so a concurrent
	// caller cannot also succeed.
	//
	// Defense-in-depth: GORM v2's Delete returns nil error even when zero
	// rows match (i.e., the row was already deleted by a concurrent
	// transaction we somehow didn't serialize against). Insist on
	// RowsAffected == 1 so a "lost the race" caller cannot silently proceed
	// to the OAuth code exchange a second time.
	res := tx.Delete(&record)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, errStateNotFound
	}
	if commitErr := tx.Commit().Error; commitErr != nil {
		return nil, commitErr
	}
	committed = true
	return &record, nil
}

// truncateUserAgent caps a User-Agent header at the storage column width
// (255). We truncate by byte rather than by rune because the column is
// VARCHAR(255) in utf8mb4 — runes are not the right unit. The few bytes of
// truncation we may perform on multi-byte UA strings is acceptable; the UA
// is only used for state binding equality, and we apply the same truncation
// at both creation and consumption time.
func truncateUserAgent(ua string) string {
	const max = 255
	if len(ua) <= max {
		return ua
	}
	return ua[:max]
}

func (h *Handler) resolveProvider(provider string) (config.OAuthProviderConfig, bool) {
	if provider == "" {
		return config.OAuthProviderConfig{}, false
	}

	var allowed bool
	if len(h.cfg.AllowedOAuthProviders) == 0 {
		allowed = true
	} else {
		for _, p := range h.cfg.AllowedOAuthProviders {
			if strings.EqualFold(p, provider) {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return config.OAuthProviderConfig{}, false
	}

	if h.cfg.OAuthProviders == nil {
		return config.OAuthProviderConfig{}, false
	}

	providerCfg, ok := h.cfg.OAuthProviders[provider]
	if ok {
		return providerCfg, true
	}

	// fall back to case-insensitive lookup
	for key, value := range h.cfg.OAuthProviders {
		if strings.EqualFold(key, provider) {
			return value, true
		}
	}
	return config.OAuthProviderConfig{}, false
}

func buildAuthURL(cfg config.OAuthProviderConfig, redirectURL, state, nonce, codeChallenge string) (string, error) {
	if cfg.AuthURL == "" {
		return "", fmt.Errorf("missing auth url for provider")
	}
	authURL, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return "", err
	}
	query := authURL.Query()
	query.Set("client_id", cfg.ClientID)
	if redirectURL != "" {
		query.Set("redirect_uri", redirectURL)
	} else if cfg.RedirectURL != "" {
		query.Set("redirect_uri", cfg.RedirectURL)
	}
	query.Set("response_type", "code")
	if len(cfg.Scopes) > 0 {
		query.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	query.Set("state", state)
	if nonce != "" {
		query.Set("nonce", nonce)
	}
	// Add PKCE parameters (RFC 7636)
	if codeChallenge != "" {
		query.Set("code_challenge", codeChallenge)
		query.Set("code_challenge_method", "S256") // SHA-256
	}
	authURL.RawQuery = query.Encode()
	return authURL.String(), nil
}

func oauthPurposeFromState(state model.UserOAuthState) model.OAuthPurpose {
	if strings.EqualFold(state.Purpose, string(model.OAuthPurposeBindLoginMethod)) {
		return model.OAuthPurposeBindLoginMethod
	}
	return model.OAuthPurposeLogin
}

func (h *Handler) authorizeBindCallback(c *gin.Context, stateRecord model.UserOAuthState) error {
	if !stateRecord.UserID.Valid || stateRecord.UserID.Int64 <= 0 {
		return errMissingBindUser
	}

	authenticatedUserID, ok := middleware.GetUserID(c)
	if !ok || authenticatedUserID == 0 {
		return errBindAuthRequired
	}
	if authenticatedUserID != uint64(stateRecord.UserID.Int64) {
		return errBindUserMismatch
	}

	middlewareService := &service.ServiceGroupApp.UserServiceGroup.MiddlewareService
	userPtr, err := middlewareService.GetUserByID(authenticatedUserID)
	if err != nil {
		return err
	}
	if userPtr == nil || userPtr.Status != model.UserStatusActive {
		return errBindAuthRequired
	}
	return nil
}

// validateScopes checks if granted scopes meet minimum requirements
// Returns warning messages if critical scopes are missing
func validateScopes(requested []string, granted string) []string {
	if len(requested) == 0 {
		return nil // No scope requirements
	}

	// Parse granted scopes (space-separated string)
	grantedMap := make(map[string]bool)
	for _, scope := range strings.Split(granted, " ") {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			grantedMap[scope] = true
		}
	}

	var warnings []string
	for _, reqScope := range requested {
		if !grantedMap[reqScope] {
			warnings = append(warnings, fmt.Sprintf("scope '%s' was requested but not granted", reqScope))
		}
	}

	return warnings
}
