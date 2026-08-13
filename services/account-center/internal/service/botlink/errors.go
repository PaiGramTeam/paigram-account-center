// Package botlink owns bot identity persistence and audit side effects.
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

	// ErrEntryIssuerMismatch rejects callers that present a namespace other
	// than the one registered for the logical bot.
	ErrEntryIssuerMismatch = errors.New("botlink: entry issuer does not match registered bot")

	// ErrUnlinkPending prevents a fresh link from overtaking revocation.
	ErrUnlinkPending = errors.New("botlink: identity unlink is still pending")
)
