package mihomostub

import (
	"context"

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

func (Client) IssueAuthKey(context.Context, string, string) (string, int64, error) {
	return "stub-authkey", 300, nil
}

var _ platformmihomo.Client = Client{}
