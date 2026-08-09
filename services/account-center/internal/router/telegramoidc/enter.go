// Package telegramoidc wires the Telegram OIDC HTTP routes onto the
// unauthenticated /api/v1 group.
//
// The two endpoints (/auth/telegram/start, /auth/telegram/callback) are
// session-establishment endpoints and therefore deliberately reachable
// without a prior session cookie. They are mounted on the public router
// alongside the legacy /auth/login + /auth/oauth/* family.
//
// Spec: docs/superpowers/specs/2026-06-06-phase5-sub1-telegram-oidc-bot-link.md §5.5
package telegramoidc

import (
	"github.com/gin-gonic/gin"

	"paigram/internal/handler"
)

// RouterGroup wires the Telegram OIDC handler onto a gin router subtree.
//
// See AGENTS.md §2 (enter.go group management): the router fetches its
// handler dependencies from the global handler.ApiGroupApp populated by
// handler.InitializeApiGroups at boot.
type RouterGroup struct{}

// InitPublic mounts GET /auth/telegram/start and GET /auth/telegram/callback
// on the UNAUTHENTICATED group. Both handlers ARE the session-establishment
// endpoints — wrapping them in AuthMiddleware would create a chicken-and-egg
// deadlock (no session cookie exists yet when the browser first hits /start).
func (r *RouterGroup) InitPublic(rg *gin.RouterGroup) {
	h := handler.ApiGroupApp.TelegramOIDCApiGroup.OIDC
	auth := rg.Group("/auth/telegram")
	{
		auth.GET("/start", h.Start)
		auth.GET("/callback", h.Callback)
	}
}
