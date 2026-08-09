package mihomo

import "context"

type DiscoveredProfile struct {
	GameBiz  string
	Region   string
	PlayerID string
	Nickname string
	Level    int32
}

type Client interface {
	ValidateAndDiscover(ctx context.Context, cookieBundleJSON string, regionHint string) (accountID string, region string, profiles []DiscoveredProfile, err error)
	IssueAuthKey(ctx context.Context, cookieBundleJSON string, playerID string) (authKey string, expiresInSeconds int64, err error)
}
