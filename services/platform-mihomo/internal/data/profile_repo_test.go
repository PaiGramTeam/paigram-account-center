package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"platform-mihomo-service/internal/biz"
)

func TestProfileRepoListByBindingIDReturnsOnlyBindingProfiles(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewProfileRepo(db)
	now := time.Now().UTC()

	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "1008611",
		Nickname:          "Traveler",
		Level:             60,
		IsDefault:         true,
		DiscoveredAt:      now,
	}))
	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         7,
		PlatformAccountID: "binding_7_30003",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "700001",
		Nickname:          "Other",
		Level:             50,
		IsDefault:         true,
		DiscoveredAt:      now,
	}))

	profiles, err := repo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Equal(t, uint64(42), profiles[0].BindingID)
	require.Equal(t, "1008611", profiles[0].PlayerID)
}

func TestProfileRepoAllowsSamePlayerAndRegionAcrossBindings(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewProfileRepo(db)
	now := time.Now().UTC()

	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "1008611",
		Nickname:          "Traveler",
		Level:             60,
		IsDefault:         true,
		DiscoveredAt:      now,
	}))
	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         7,
		PlatformAccountID: "binding_7_10001",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "1008611",
		Nickname:          "Traveler Alt",
		Level:             55,
		IsDefault:         true,
		DiscoveredAt:      now,
	}))

	first, err := repo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, "Traveler", first[0].Nickname)

	second, err := repo.ListByBindingID(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, "Traveler Alt", second[0].Nickname)
}

func TestProfileRepoRejectsMultipleDefaultProfilesPerBinding(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewProfileRepo(db)
	now := time.Now().UTC()

	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "1008611",
		Nickname:          "Traveler",
		Level:             60,
		IsDefault:         true,
		DiscoveredAt:      now,
	}))

	err := repo.Save(context.Background(), &biz.Profile{
		BindingID:         42,
		PlatformAccountID: "binding_42_20002",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "2000222",
		Nickname:          "Traveler Alt",
		Level:             55,
		IsDefault:         true,
		DiscoveredAt:      now,
	})
	require.True(t, errors.Is(err, ErrDefaultProfileAlreadyExists), "got error %v", err)

	profiles, err := repo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Equal(t, "1008611", profiles[0].PlayerID)
	require.Equal(t, "Traveler", profiles[0].Nickname)
}

func TestProfileRepoAllowsUpdatingSameDefaultProfile(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewProfileRepo(db)
	now := time.Now().UTC()

	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "1008611",
		Nickname:          "Traveler",
		Level:             60,
		IsDefault:         true,
		DiscoveredAt:      now,
	}))

	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "1008611",
		Nickname:          "Traveler Updated",
		Level:             61,
		IsDefault:         true,
		DiscoveredAt:      now,
	}))

	profiles, err := repo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Equal(t, "Traveler Updated", profiles[0].Nickname)
	require.Equal(t, 61, profiles[0].Level)
	require.True(t, profiles[0].IsDefault)
}

func TestProfileRepoSetDefaultSwitchesProfiles(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewProfileRepo(db)
	now := time.Now().UTC()

	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "1008611",
		Nickname:          "Traveler",
		Level:             60,
		IsDefault:         true,
		DiscoveredAt:      now,
	}))
	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "2000222",
		Nickname:          "Traveler Alt",
		Level:             55,
		IsDefault:         false,
		DiscoveredAt:      now,
	}))

	require.NoError(t, repo.SetDefaultByBindingAndPlayerID(context.Background(), 42, "binding_42_10001", "2000222"))

	profiles, err := repo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	require.False(t, profiles[0].IsDefault)
	require.True(t, profiles[1].IsDefault)
}

func TestProfileRepoSetDefaultClearsExistingDefaultAcrossPlatformAccounts(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewProfileRepo(db)
	now := time.Now().UTC()

	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         42,
		PlatformAccountID: "binding_42_10001",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "1008611",
		Nickname:          "Traveler",
		Level:             60,
		IsDefault:         true,
		DiscoveredAt:      now,
	}))
	require.NoError(t, repo.Save(context.Background(), &biz.Profile{
		BindingID:         42,
		PlatformAccountID: "binding_42_20002",
		GameBiz:           "hk4e_cn",
		Region:            "cn_gf01",
		PlayerID:          "2000222",
		Nickname:          "Traveler Alt",
		Level:             55,
		IsDefault:         false,
		DiscoveredAt:      now,
	}))

	require.NoError(t, repo.SetDefaultByBindingAndPlayerID(context.Background(), 42, "binding_42_20002", "2000222"))

	profiles, err := repo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, profiles, 2)

	defaultCount := 0
	var defaultPlayerID string
	for _, profile := range profiles {
		if profile.IsDefault {
			defaultCount++
			defaultPlayerID = profile.PlayerID
		}
	}
	require.Equal(t, 1, defaultCount)
	require.Equal(t, "2000222", defaultPlayerID)
}

func TestProfileRepoSetDefaultReturnsNotFoundForMissingProfile(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewProfileRepo(db)

	err := repo.SetDefaultByBindingAndPlayerID(context.Background(), 42, "binding_42_10001", "missing")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
