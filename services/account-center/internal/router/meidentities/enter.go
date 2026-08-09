// Package meidentities wires the linked-Telegram-identities HTTP routes
// onto the session-authenticated /api/v1/me/* group.
//
// AuthMiddleware (post-A4.1-A, see internal/middleware/auth.go) reads
// either `Authorization: Bearer <token>` or the `ac_session` cookie, so
// OIDC-issued sessions (A4 + A4.1-C) reach this handler with userID set
// without additional middleware glue.
//
// Spec: docs/superpowers/specs/2026-06-06-phase5-sub1-telegram-oidc-bot-link.md §5.5
package meidentities

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"paigram/internal/handler"
)

// RouterGroup wires the meidentities handler onto a gin router subtree.
// Follows the AGENTS.md §2 enter.go pattern used by sibling /me routers.
type RouterGroup struct{}

// Init mounts the /me/bot-identities subtree on the SESSION-AUTHENTICATED
// group passed in by router.InitializeRouterGroups.
//
// The db argument is unused — kept in the signature so the wiring matches
// the sibling Init(rg, db) shape that the aggregating
// router.InitializeRouterGroups uses for every protected router group.
func (r *RouterGroup) Init(rg *gin.RouterGroup, _ *gorm.DB) {
	h := handler.ApiGroupApp.MeIdentitiesApiGroup.Identities
	identities := rg.Group("/me/bot-identities")
	{
		identities.GET("", h.List)
		identities.DELETE("/:botId", h.Unlink)
	}
}
