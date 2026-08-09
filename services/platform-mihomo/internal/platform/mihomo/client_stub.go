package mihomo

import (
	"context"
	"errors"
)

type StubClient struct{}

type UnconfiguredClient struct{}

func (StubClient) ValidateAndDiscover(ctx context.Context, cookieBundleJSON string, regionHint string) (string, string, []DiscoveredProfile, error) {
	return "10001", "cn_gf01", []DiscoveredProfile{{
		GameBiz:  "hk4e_cn",
		Region:   "cn_gf01",
		PlayerID: "1008611",
		Nickname: "Traveler",
		Level:    60,
	}}, nil
}

func (StubClient) IssueAuthKey(ctx context.Context, cookieBundleJSON string, playerID string) (string, int64, error) {
	return "stub-authkey", 300, nil
}

func (UnconfiguredClient) ValidateAndDiscover(ctx context.Context, cookieBundleJSON string, regionHint string) (string, string, []DiscoveredProfile, error) {
	return "", "", nil, errors.New("mihomo upstream client not configured")
}

func (UnconfiguredClient) IssueAuthKey(ctx context.Context, cookieBundleJSON string, playerID string) (string, int64, error) {
	return "", 0, errors.New("mihomo upstream client not configured")
}
