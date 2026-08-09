package telegramoidc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"paigram/internal/response"
	"paigram/internal/service/botlink"
	"paigram/internal/service/session"
	telegramoidcsvc "paigram/internal/service/telegramoidc"
	"paigram/internal/service/user"
)

const (
	// defaultRedirectTo is the post-login destination when /start is
	// invoked without an explicit ?redirect_to=. Matches spec §5.3.
	defaultRedirectTo = "/me/bot-identities"

	// errorPagePath is the user-app error page; reason codes follow
	// spec §7.1.
	errorPagePath = "/auth/telegram/error"

	// provider is the credential provider key persisted on
	// user_credentials.provider for users provisioned through this flow.
	// "telegram-oidc" disambiguates from the legacy
	// handler/auth.HandleOAuthCallback telegram path which writes
	// provider="telegram".
	provider = "telegram-oidc"
)

// Reason codes — spec §7.1. Centralised as constants so the
// errorRedirect helper cannot drift from the user-facing message map.
const (
	reasonStateInvalid        = "state_invalid"
	reasonUpstreamUnavailable = "upstream_unavailable"
	reasonTokenInvalid        = "token_invalid"
	reasonMissingTelegramID   = "missing_telegram_id"
	reasonUserDenied          = "user_denied"
	reasonAlreadyLinkedOther  = "already_linked_other"
	reasonUserAlreadyLinked   = "user_already_linked"
	reasonInternal            = "internal"
)

// Handler wires the OIDC client, state store, user provisioning, session
// issuance, and bot_identities upsert into the /auth/telegram routes.
//
// The handler owns the *gorm.DB used to open the outer transaction that
// wraps Callback's steps 9a-9f per spec §6.3. Services it calls into
// (state, userSvc, botlink, sessionSvc) expose `*Tx` variants that run
// against this transaction; this handler is the only owner of the wrap.
type Handler struct {
	db         *gorm.DB
	oidc       *telegramoidcsvc.Client
	state      *telegramoidcsvc.StateStore
	userSvc    *user.UserService
	sessionSvc *session.Service
	botlink    *botlink.Service
	logger     *zap.Logger
}

// NewHandler builds a Handler. logger must be non-nil; pass zap.NewNop()
// in tests that don't care about log output. db is the same connection
// used by the service constructors below; the Callback handler opens
// its outer transaction off this *gorm.DB and threads the resulting
// `*gorm.DB` tx into each service's `*Tx` method.
func NewHandler(
	db *gorm.DB,
	oidc *telegramoidcsvc.Client,
	state *telegramoidcsvc.StateStore,
	userSvc *user.UserService,
	sessionSvc *session.Service,
	botlinkSvc *botlink.Service,
	logger *zap.Logger,
) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{
		db:         db,
		oidc:       oidc,
		state:      state,
		userSvc:    userSvc,
		sessionSvc: sessionSvc,
		botlink:    botlinkSvc,
		logger:     logger,
	}
}

// Start initiates the Telegram OIDC flow.
//
//	@Summary  Start Telegram OIDC login
//	@Tags     auth-telegram
//	@Param    bot          query  string  true   "Bot identifier (e.g. paigrambot)"
//	@Param    redirect_to  query  string  false  "Post-login destination, default /me/bot-identities"
//	@Success  302
//	@Failure  400  {object}  response.Response
//	@Router   /auth/telegram/start [get]
func (h *Handler) Start(c *gin.Context) {
	botID := strings.TrimSpace(c.Query("bot"))
	if botID == "" {
		response.BadRequest(c, "bot query parameter required")
		return
	}
	redirectTo := c.DefaultQuery("redirect_to", defaultRedirectTo)

	verifier, challenge, err := newPKCE()
	if err != nil {
		h.logger.Error("telegramoidc: pkce generation failed",
			zap.String("bot_id", botID), zap.Error(err))
		h.errorRedirect(c, reasonInternal)
		return
	}

	state, err := h.state.Issue(c.Request.Context(), telegramoidcsvc.IssueInput{
		CodeVerifier: verifier,
		BotID:        botID,
		RedirectTo:   redirectTo,
		ClientIP:     c.ClientIP(),
		UserAgent:    c.GetHeader("User-Agent"),
	})
	if err != nil {
		h.logger.Error("telegramoidc: state issue failed",
			zap.String("bot_id", botID), zap.Error(err))
		h.errorRedirect(c, reasonInternal)
		return
	}

	c.Redirect(http.StatusFound, h.oidc.AuthorizeURL(state, challenge))
}

// callbackError is the sentinel type the Callback transaction returns so
// the outer switch can map (logical step, underlying error) → reason
// code without re-evaluating each step's error inline. The wrapped
// errors.Is checks still work via the embedded `err`.
type callbackError struct {
	step string
	err  error
}

func (e *callbackError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.step + ": " + e.err.Error()
}

func (e *callbackError) Unwrap() error { return e.err }

// Callback completes the OIDC flow and atomically links bot_identities.
//
//	@Summary  Telegram OIDC callback
//	@Tags     auth-telegram
//	@Param    code   query  string  false  "Authorization code (success path)"
//	@Param    state  query  string  true   "State token from Start"
//	@Param    error  query  string  false  "OIDC error (denial path)"
//	@Success  302
//	@Router   /auth/telegram/callback [get]
func (h *Handler) Callback(c *gin.Context) {
	rawState := strings.TrimSpace(c.Query("state"))
	if rawState == "" {
		h.errorRedirect(c, reasonStateInvalid)
		return
	}

	ctx := c.Request.Context()

	// rec is captured into the outer closure so the success-path 302
	// (after the transaction commits) can read rec.RedirectTo. On a
	// failed transaction it stays nil and the error switch routes to
	// the appropriate reason= page.
	var rec *telegramoidcsvc.StateRecord

	// consumedReason flags an "input was bad but state IS validly
	// consumed" outcome — user_denied and missing_code per spec §6.2
	// row 9. The TX commits (so the state row's consumed_at sticks and
	// the user cannot replay) but the handler still redirects to the
	// reason= error page. Distinct from txErr which signals a true
	// rollback.
	var consumedReason string

	// Single GORM transaction wrapping spec §6.3 steps 9a, 9d, 9e, 9f.
	// External I/O (steps 9b-9c: token exchange + JWT verify) runs
	// INSIDE the transaction between Consume and FindOrCreate so a
	// downstream failure (e.g. session.Issue) rolls the state's
	// consumed_at marker back. The trade-off: the state row's UPDATE
	// lock is held across the oauth.telegram.org HTTP RTT (sub-second).
	// TODO(post-prod): re-evaluate whether to bracket external I/O
	// outside the TX once we have real latency data — see spec §6.3.
	txErr := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		rec, err = h.state.ConsumeTx(ctx, tx, telegramoidcsvc.ConsumeInput{
			State:     rawState,
			ClientIP:  c.ClientIP(),
			UserAgent: c.GetHeader("User-Agent"),
		})
		if err != nil {
			return &callbackError{step: "consume", err: err}
		}

		// User-denied path. Spec §6.2 row 9: consume the state (already
		// done above for anti-retry) and redirect with reason=user_denied.
		// Return nil so the TX commits — the state's consumed_at marker
		// MUST persist or the user could replay. The outer code reads
		// consumedReason to drive the redirect.
		if oidcErr := strings.TrimSpace(c.Query("error")); oidcErr != "" {
			h.logger.Info("telegramoidc: user denied",
				zap.String("oidc_error", oidcErr),
				zap.String("bot_id", rec.BotID))
			consumedReason = reasonUserDenied
			return nil
		}

		code := strings.TrimSpace(c.Query("code"))
		if code == "" {
			h.logger.Warn("telegramoidc: missing code on success-path callback",
				zap.String("bot_id", rec.BotID))
			consumedReason = reasonTokenInvalid
			return nil
		}

		token, err := h.oidc.ExchangeCode(ctx, code, rec.CodeVerifier)
		if err != nil {
			return &callbackError{step: "exchange", err: err}
		}

		claims, err := h.oidc.VerifyIDToken(ctx, token.IDToken)
		if err != nil {
			return &callbackError{step: "verify", err: err}
		}

		userID, err := h.userSvc.FindOrCreateOIDCTx(ctx, tx, provider, claims.Sub, claims.Name)
		if err != nil {
			return &callbackError{step: "user", err: err}
		}

		var usernamePtr *string
		if u := strings.TrimSpace(claims.PreferredUsername); u != "" {
			usernamePtr = &u
		}
		if _, err := h.botlink.UpsertLinkTx(ctx, tx, botlink.UpsertLinkInput{
			BotID:            rec.BotID,
			UserID:           userID,
			ExternalUserID:   strconv.FormatInt(claims.ID, 10),
			ExternalUsername: usernamePtr,
			RequestIP:        c.ClientIP(),
			RequestUA:        c.Request.UserAgent(),
		}); err != nil {
			return &callbackError{step: "upsert", err: err}
		}

		// session.IssueTx MUST be the last step — the Set-Cookie header
		// is queued on c.Writer before this returns, so a subsequent
		// rollback would orphan a cookie. See IssueTx godoc.
		if err := h.sessionSvc.IssueTx(tx, c, userID); err != nil {
			return &callbackError{step: "session", err: err}
		}
		return nil
	})

	if txErr != nil {
		h.routeCallbackError(c, txErr, rec)
		return
	}
	if consumedReason != "" {
		// State was validly consumed but the user input drove us to an
		// error page (denial / missing code). The TX committed, so
		// consumed_at is persisted and the user cannot replay.
		h.errorRedirect(c, consumedReason)
		return
	}

	c.Redirect(http.StatusFound, rec.RedirectTo)
}

// routeCallbackError maps a transaction error returned by Callback's
// closure to the appropriate reason= 302. Centralised so the
// (step, underlying error) → reason mapping is auditable in one place.
func (h *Handler) routeCallbackError(c *gin.Context, txErr error, rec *telegramoidcsvc.StateRecord) {
	var cbErr *callbackError
	if !errors.As(txErr, &cbErr) {
		h.logger.Error("telegramoidc: callback tx failed (unwrapped)", zap.Error(txErr))
		h.errorRedirect(c, reasonInternal)
		return
	}

	// Some logs want bot_id from the captured state record; it is nil
	// when the consume step itself failed (state never resolved).
	var botID string
	if rec != nil {
		botID = rec.BotID
	}

	switch cbErr.step {
	case "consume":
		// state_store.ConsumeTx returns ErrInvalidState for missing,
		// expired, already-consumed, or client-binding-mismatched rows.
		// The opaque sentinel is intentional — see state_store.go.
		h.logger.Warn("telegramoidc: state consume failed", zap.Error(cbErr.err))
		h.errorRedirect(c, reasonStateInvalid)
	case "exchange":
		switch {
		case errors.Is(cbErr.err, telegramoidcsvc.ErrTokenExchangeRejected):
			h.logger.Warn("telegramoidc: token exchange rejected",
				zap.String("bot_id", botID), zap.Error(cbErr.err))
			h.errorRedirect(c, reasonTokenInvalid)
		default:
			h.logger.Error("telegramoidc: token exchange failed",
				zap.String("bot_id", botID), zap.Error(cbErr.err))
			h.errorRedirect(c, reasonUpstreamUnavailable)
		}
	case "verify":
		switch {
		case errors.Is(cbErr.err, telegramoidcsvc.ErrTelegramIDMissing):
			h.logger.Error("telegramoidc: id_token missing telegram id",
				zap.String("bot_id", botID), zap.Error(cbErr.err))
			h.errorRedirect(c, reasonMissingTelegramID)
		case errors.Is(cbErr.err, telegramoidcsvc.ErrJWKSUnavailable):
			h.logger.Error("telegramoidc: jwks unavailable",
				zap.String("bot_id", botID), zap.Error(cbErr.err))
			h.errorRedirect(c, reasonUpstreamUnavailable)
		default:
			h.logger.Error("telegramoidc: id_token verification failed",
				zap.String("bot_id", botID), zap.Error(cbErr.err))
			h.errorRedirect(c, reasonTokenInvalid)
		}
	case "user":
		h.logger.Error("telegramoidc: user provision failed",
			zap.String("bot_id", botID),
			zap.Error(cbErr.err))
		h.errorRedirect(c, reasonInternal)
	case "upsert":
		switch {
		case errors.Is(cbErr.err, botlink.ErrTelegramAlreadyLinkedToOther):
			h.logger.Warn("telegramoidc: telegram identity owned by another user",
				zap.String("bot_id", botID))
			h.errorRedirect(c, reasonAlreadyLinkedOther)
		case errors.Is(cbErr.err, botlink.ErrAlreadyLinked):
			h.logger.Warn("telegramoidc: user already linked to a different telegram identity",
				zap.String("bot_id", botID))
			h.errorRedirect(c, reasonUserAlreadyLinked)
		default:
			h.logger.Error("telegramoidc: botlink upsert failed",
				zap.String("bot_id", botID),
				zap.Error(cbErr.err))
			h.errorRedirect(c, reasonInternal)
		}
	case "session":
		h.logger.Error("telegramoidc: session issue failed", zap.Error(cbErr.err))
		h.errorRedirect(c, reasonInternal)
	default:
		h.logger.Error("telegramoidc: callback tx failed (unknown step)",
			zap.String("step", cbErr.step), zap.Error(cbErr.err))
		h.errorRedirect(c, reasonInternal)
	}
}

// --- helpers ---

// errorRedirect issues a 302 to errorPagePath?reason=<code>. Centralising
// this prevents accidental string concatenation that could let user input
// influence the redirect target.
func (h *Handler) errorRedirect(c *gin.Context, reason string) {
	target := errorPagePath + "?" + url.Values{"reason": {reason}}.Encode()
	c.Redirect(http.StatusFound, target)
}

// newPKCE generates an RFC 7636 code verifier (43 URL-safe base64 chars
// from 32 random bytes) and its S256 challenge.
func newPKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}
