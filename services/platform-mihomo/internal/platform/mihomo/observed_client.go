package mihomo

import (
	"context"
	"errors"
	"time"
)

type UpstreamObserver interface {
	RecordUpstreamResult(operation string, degraded bool)
}

type ObservedClient struct {
	inner    Client
	observer UpstreamObserver
}

func NewObservedClient(inner Client, observer UpstreamObserver) *ObservedClient {
	return &ObservedClient{inner: inner, observer: observer}
}

func (c *ObservedClient) ValidateAndDiscover(ctx context.Context, credential, region string) (string, string, []DiscoveredProfile, error) {
	accountID, resolvedRegion, profiles, err := c.inner.ValidateAndDiscover(ctx, credential, region)
	c.observe("discover", err)
	return accountID, resolvedRegion, profiles, err
}

func (c *ObservedClient) RefreshCredential(ctx context.Context, credential, region string) (RefreshResult, error) {
	refresher, ok := c.inner.(CredentialRefresher)
	if !ok {
		err := &UpstreamError{Kind: ErrorUnavailable}
		c.observe("refresh", err)
		return RefreshResult{}, err
	}
	result, err := refresher.RefreshCredential(ctx, credential, region)
	c.observe("refresh", err)
	return result, err
}

func (c *ObservedClient) IssueAuthKey(ctx context.Context, credential, playerID string) (string, int64, error) {
	authKey, expiresIn, err := c.inner.IssueAuthKey(ctx, credential, playerID)
	c.observe("issue_authkey", err)
	return authKey, expiresIn, err
}

func (c *ObservedClient) IssueAuthKeyWithTTL(ctx context.Context, credential, playerID string, ttl time.Duration) (string, int64, error) {
	issuer, ok := c.inner.(BoundedAuthKeyIssuer)
	if !ok {
		err := &UpstreamError{Kind: ErrorUnavailable}
		c.observe("issue_authkey", err)
		return "", 0, err
	}
	authKey, expiresIn, err := issuer.IssueAuthKeyWithTTL(ctx, credential, playerID, ttl)
	c.observe("issue_authkey", err)
	return authKey, expiresIn, err
}

func (c *ObservedClient) RevokeAuthKey(ctx context.Context, authKey string) error {
	revoker, ok := c.inner.(AuthKeyRevoker)
	if !ok {
		err := &UpstreamError{Kind: ErrorUnavailable}
		c.observe("revoke_authkey", err)
		return err
	}
	err := revoker.RevokeAuthKey(ctx, authKey)
	c.observe("revoke_authkey", err)
	return err
}

func (c *ObservedClient) observe(operation string, err error) {
	if c.observer == nil {
		return
	}
	var upstreamErr *UpstreamError
	degraded := errors.As(err, &upstreamErr) && (upstreamErr.Kind == ErrorUnavailable || upstreamErr.Kind == ErrorRateLimited || upstreamErr.Kind == ErrorInvalidResponse)
	c.observer.RecordUpstreamResult(operation, degraded)
}
