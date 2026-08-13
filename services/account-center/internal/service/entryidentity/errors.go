package entryidentity

import "errors"

var (
	ErrInvalidInput         = errors.New("entry identity link input is invalid")
	ErrPrincipalMismatch    = errors.New("entry identity principal mapping is invalid")
	ErrNamespaceUnavailable = errors.New("entry identity namespace is unavailable")
	ErrChallengeNotFound    = errors.New("entry identity link challenge not found")
	ErrChallengeExpired     = errors.New("entry identity link challenge expired")
	ErrChallengeConsumed    = errors.New("entry identity link challenge already consumed")
	ErrIdentityConflict     = errors.New("entry identity already belongs to another user")
	ErrChallengeCapacity    = errors.New("too many pending entry identity challenges")
	ErrUnlinkPending        = errors.New("entry identity unlink propagation is pending")
)
