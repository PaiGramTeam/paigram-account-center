package mihomostub

import (
	"context"
	"time"

	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

type Client struct {
	Profiles []platformmihomo.DiscoveredProfile
}

func (c Client) ValidateAndDiscover(context.Context, string, string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	if len(c.Profiles) > 0 {
		return "10001", "cn_gf01", c.Profiles, nil
	}
	return "10001", "cn_gf01", []platformmihomo.DiscoveredProfile{{
		GameBiz:  "hk4e_cn",
		Region:   "cn_gf01",
		PlayerID: "1008611",
		Nickname: "Traveler",
		Level:    60,
	}}, nil
}

func (c Client) RefreshCredential(_ context.Context, cookieBundleJSON string, _ string) (platformmihomo.RefreshResult, error) {
	profiles := c.Profiles
	if len(profiles) == 0 {
		_, _, profiles, _ = c.ValidateAndDiscover(context.Background(), cookieBundleJSON, "")
	}
	return platformmihomo.RefreshResult{
		CredentialBundleJSON: cookieBundleJSON,
		AccountID:            "10001",
		Region:               "cn_gf01",
		Profiles:             profiles,
		ExpiresAt:            time.Now().UTC().Add(time.Hour),
	}, nil
}

func (Client) IssueAuthKey(context.Context, string, string) (string, int64, error) {
	return "stub-authkey", 300, nil
}

func (Client) IssueAuthKeyWithTTL(_ context.Context, _ string, _ string, ttl time.Duration) (string, int64, error) {
	return "stub-authkey", int64(ttl / time.Second), nil
}

func (Client) RevokeAuthKey(context.Context, string) error {
	return nil
}

var _ platformmihomo.Client = Client{}
