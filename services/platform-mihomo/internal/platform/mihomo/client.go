package mihomo

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type ErrorKind string

const (
	ErrorInvalidCredential ErrorKind = "invalid_credential"
	ErrorExpiredCredential ErrorKind = "expired_credential"
	ErrorChallengeRequired ErrorKind = "challenge_required"
	ErrorRateLimited       ErrorKind = "rate_limited"
	ErrorUnavailable       ErrorKind = "unavailable"
	ErrorInvalidResponse   ErrorKind = "invalid_response"
)

type UpstreamError struct {
	Kind       ErrorKind
	StatusCode int
	RetryAfter time.Duration
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "mihomo upstream error"
	}
	return fmt.Sprintf("mihomo upstream request failed: %s", e.Kind)
}

func IsErrorKind(err error, kind ErrorKind) bool {
	var upstreamErr *UpstreamError
	return errors.As(err, &upstreamErr) && upstreamErr.Kind == kind
}

type DiscoveredProfile struct {
	GameBiz  string
	Region   string
	PlayerID string
	Nickname string
	Level    int32
}

type RefreshResult struct {
	CredentialBundleJSON string
	AccountID            string
	Region               string
	Profiles             []DiscoveredProfile
	ExpiresAt            time.Time
}

type CredentialRefresher interface {
	RefreshCredential(ctx context.Context, cookieBundleJSON string, regionHint string) (RefreshResult, error)
}

type Client interface {
	ValidateAndDiscover(ctx context.Context, cookieBundleJSON string, regionHint string) (accountID string, region string, profiles []DiscoveredProfile, err error)
	IssueAuthKey(ctx context.Context, cookieBundleJSON string, playerID string) (authKey string, expiresInSeconds int64, err error)
}
