// Package botlink owns bot_identities CRUD with audit-log side effects.
// Spec: docs/superpowers/specs/2026-06-06-phase5-sub1-telegram-oidc-bot-link.md §5.2
package botlink

import "errors"

var (
	// ErrAlreadyLinked: (user_id, bot_id) UNIQUE collision.
	// Caller is the same user trying to link a different Telegram account
	// to a bot they already have linked.
	ErrAlreadyLinked = errors.New("botlink: user already linked to this bot")

	// ErrTelegramAlreadyLinkedToOther: (bot_id, external_user_id) UNIQUE
	// collision where the existing row belongs to a different user.
	ErrTelegramAlreadyLinkedToOther = errors.New("botlink: telegram id linked to other user")

	// ErrBotIdentityNotFound: Unlink target row does not exist.
	ErrBotIdentityNotFound = errors.New("botlink: bot identity not found")
)
