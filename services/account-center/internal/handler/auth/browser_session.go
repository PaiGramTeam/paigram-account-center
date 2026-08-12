package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"paigram/internal/logging"
	"paigram/internal/model"
	"paigram/internal/response"
)

const (
	refreshCookieName = "ac_refresh"
	refreshCookiePath = "/api/v1/auth"
)

var errInvalidBrowserOrigin = errors.New("invalid browser origin")

func (h *Handler) setBrowserRefreshCookie(c *gin.Context, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		MaxAge:   maxAge,
		Expires:  expiresAt,
		Secure:   h.cfg.SessionCookieSecure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearBrowserRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Path:     refreshCookiePath,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
		Secure:   h.cfg.SessionCookieSecure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) browserRefreshToken(c *gin.Context) (string, error) {
	if err := h.validateBrowserOrigin(c); err != nil {
		return "", err
	}
	token, err := c.Cookie(refreshCookieName)
	if err != nil || strings.TrimSpace(token) == "" {
		return "", http.ErrNoCookie
	}
	return token, nil
}

func (h *Handler) logoutToken(c *gin.Context) (string, bool, error) {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if authorization != "" {
		parts := strings.SplitN(authorization, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" && strings.TrimSpace(parts[1]) != "" {
			return strings.TrimSpace(parts[1]), false, nil
		}
		return "", false, errors.New("invalid authorization header")
	}
	token, err := h.browserRefreshToken(c)
	return token, true, err
}

func (h *Handler) rejectRefreshReplay(c *gin.Context, tokenHash string, now time.Time) bool {
	var history model.UserRefreshTokenHistory
	if err := h.db.Where("token_hash = ? AND expires_at > ?", tokenHash, now).First(&history).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false
		}
		logging.Error("failed to inspect refresh token history", zap.Error(err))
		response.InternalServerError(c, "failed to inspect refresh session")
		return true
	}
	if !h.revokeRefreshFamily(c, history.FamilyID, "token_reuse_detected", now) {
		return true
	}
	h.clearBrowserRefreshCookie(c)
	response.UnauthorizedWithCode(c, "TOKEN_REUSE_DETECTED", "security violation: refresh token reuse detected", nil)
	return true
}

func (h *Handler) revokeHistoricalRefreshToken(tokenHash, reason string, now time.Time) bool {
	var history model.UserRefreshTokenHistory
	if err := h.db.Where("token_hash = ?", tokenHash).First(&history).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logging.Error("failed to inspect refresh token history during logout", zap.Error(err))
		}
		return false
	}
	return h.revokeRefreshFamilyWithoutResponse(history.FamilyID, reason, now) == nil
}

func (h *Handler) revokeRefreshFamily(c *gin.Context, familyID, reason string, now time.Time) bool {
	if err := h.revokeRefreshFamilyWithoutResponse(familyID, reason, now); err != nil {
		logging.Error("failed to revoke refresh token family", zap.Error(err), zap.String("family_id", familyID))
		response.InternalServerError(c, "failed to revoke session family")
		return false
	}
	return true
}

func (h *Handler) revokeRefreshFamilyWithoutResponse(familyID, reason string, now time.Time) error {
	if strings.TrimSpace(familyID) == "" {
		return errors.New("refresh token family id is empty")
	}
	var sessions []model.UserSession
	if err := h.db.Where("family_id = ? AND revoked_at IS NULL", familyID).Find(&sessions).Error; err != nil {
		return err
	}
	if err := h.db.Model(&model.UserSession{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Updates(map[string]interface{}{"revoked_at": now, "revoked_reason": reason}).Error; err != nil {
		return err
	}
	h.invalidateRevokedSessionsCache(context.Background(), sessions)
	return nil
}

func (h *Handler) validateBrowserOrigin(c *gin.Context) error {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin == "" {
		if h.cfg.SessionCookieSecure {
			return errInvalidBrowserOrigin
		}
		return nil
	}

	expected, err := url.Parse(strings.TrimSpace(h.frontendCfg.BaseURL))
	if err != nil || expected.Scheme == "" || expected.Host == "" {
		return errInvalidBrowserOrigin
	}
	actual, err := url.Parse(origin)
	if err != nil || !strings.EqualFold(actual.Scheme, expected.Scheme) || !strings.EqualFold(actual.Host, expected.Host) {
		return errInvalidBrowserOrigin
	}
	return nil
}
