package botaccess

import "errors"

var (
	ErrBotIdentityNotFound       = errors.New("bot identity not found")
	ErrPlatformAccountMissing    = errors.New("platform account binding not found")
	ErrConsumerGrantNotFound     = errors.New("consumer grant not found")
	ErrConsumerGrantRevoked      = errors.New("consumer grant revoked")
	ErrConsumerNotSupported      = errors.New("consumer is not supported")
	ErrScopeNotGranted           = errors.New("requested scope is not granted")
	ErrInvalidTicketConfig       = errors.New("invalid service ticket config")
	ErrSigningKeyUnavailable     = errors.New("service ticket signing key unavailable")
	ErrInactiveAccountRef        = errors.New("platform account binding is not active")
	ErrPlatformServiceNotEnabled = errors.New("platform service is not enabled for platform")
)
