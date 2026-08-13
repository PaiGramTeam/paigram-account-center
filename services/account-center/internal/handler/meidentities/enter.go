// Package meidentities serves the authenticated user's bot_identities view
// (GET /me/bot-identities) and unlink endpoint (DELETE /me/bot-identities/:botId).
// See AGENTS.md §2 for the enter.go group-management pattern.
package meidentities

import (
	"go.uber.org/zap"

	"paigram/internal/service/botlink"
	"paigram/internal/service/entryidentity"
)

// ApiGroup is the per-package handler group registered by the router layer.
type ApiGroup struct {
	Identities *Handler
}

// NewApiGroup wires the meidentities handler with its botlink.Service dependency.
func NewApiGroup(svc *botlink.Service, logger *zap.Logger, linking ...*entryidentity.Service) *ApiGroup {
	return &ApiGroup{Identities: NewHandler(svc, logger, linking...)}
}
