package platformbinding

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildProfileProjectionInputsPrefersStableProfileReference(t *testing.T) {
	profiles := buildProfileProjectionInputs("mihomo", []map[string]any{{
		"profile_ref": "profile-stable-42",
		"id":          uint64(7),
		"player_id":   "10001",
		"game_biz":    "hk4e_cn",
		"region":      "cn_gf01",
		"nickname":    "Traveler",
	}})

	require.Len(t, profiles, 1)
	require.Equal(t, "profile-stable-42", profiles[0].PlatformProfileKey)
}
