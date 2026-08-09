// Package telegramoidc handles HTTP requests for the Telegram OIDC login
// flow and the bot_identities link side effect.
//
// AGENTS.md §2 ("enter.go group management") requires every handler
// subpackage to expose its handlers through an ApiGroup constructed in
// enter.go. The router wires the group; the group instantiates its
// handlers with the shared service dependencies.
//
// Spec: docs/superpowers/specs/2026-06-06-phase5-sub1-telegram-oidc-bot-link.md §5.3
package telegramoidc

import (
	"go.uber.org/zap"
	"gorm.io/gorm"

	"paigram/internal/service/botlink"
	"paigram/internal/service/session"
	telegramoidcsvc "paigram/internal/service/telegramoidc"
	"paigram/internal/service/user"
)

// ApiGroup is the per-package handler group registered by the router layer.
type ApiGroup struct {
	OIDC *Handler
}

// NewApiGroup wires the Telegram OIDC handler with its shared service
// dependencies. The router calls this once at boot and mounts
// OIDC.Start / OIDC.Callback on the unauthenticated /api/v1 group. The
// db argument is the same *gorm.DB the service constructors received;
// Callback opens its outer transaction off this handle per spec §6.3.
func NewApiGroup(
	db *gorm.DB,
	oidcClient *telegramoidcsvc.Client,
	stateStore *telegramoidcsvc.StateStore,
	userSvc *user.UserService,
	sessionSvc *session.Service,
	botlinkSvc *botlink.Service,
	logger *zap.Logger,
) *ApiGroup {
	return &ApiGroup{
		OIDC: NewHandler(db, oidcClient, stateStore, userSvc, sessionSvc, botlinkSvc, logger),
	}
}
