package credentials

import "errors"

// Sentinel errors returned by the credentials package. Keep these stable —
// callers (handler, interceptor, gRPC layer) match on them via errors.Is to
// build user-facing error responses.
var (
	ErrCredentialNotFound  = errors.New("service credential not found")
	ErrCredentialDisabled  = errors.New("service credential disabled")
	ErrInvalidClientSecret = errors.New("invalid client secret")
	ErrInvalidAudience     = errors.New("invalid audience for credential")
	ErrInsufficientScope   = errors.New("insufficient scope")
	ErrEmptyClientID       = errors.New("client_id must not be empty")
	ErrCredentialConflict  = errors.New("service credential already exists")
	ErrInvalidEntryIssuer  = errors.New("invalid entry identity issuer")
	ErrBotIssuerConflict   = errors.New("logical bot entry identity issuer conflicts with registry")
	// ErrInvalidStatus is returned when SetStatus receives a value
	// outside {active, disabled}. Distinct from ErrCredentialNotFound
	// so callers can map it to 400 rather than 404.
	ErrInvalidStatus = errors.New("invalid credential status")
)
