// Package botroute owns the (bot_id, platform) routing registry that
// telegram-service uses to dispatch incoming updates to the correct game
// service. The service layer enforces ownership rules and writes append-only
// audit rows; gRPC and admin handlers compose on top of it.
package botroute

import (
	"context"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service is the business layer for bot_routes. It is stateless; all state
// lives in the database. Callers are expected to long-live a single Service
// per process.
type Service struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewService creates a Service bound to the given db. A nil logger is
// replaced with a no-op logger so callers never have to nil-check.
func NewService(db *gorm.DB, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{db: db, logger: logger}
}

// actorCtxKey is the unexported key used by WithActor / actorFromContext.
// Using an unexported struct type keys avoids the well-known collision risk
// of using bare strings.
type actorCtxKey struct{}

// WithActor returns a child context carrying the actor identifier (typically
// a bot_id) for audit attribution. Empty actor values are still propagated;
// callers may decide whether to default downstream.
func WithActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, actorCtxKey{}, actor)
}

// actorFromContext extracts the actor identifier previously stored by
// WithActor. The empty string is returned when no actor is bound.
func actorFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(actorCtxKey{}).(string); ok {
		return v
	}
	return ""
}
