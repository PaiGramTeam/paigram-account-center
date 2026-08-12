package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
)

func TestSetPrimaryProfileByRefUsesStableProfileIdentity(t *testing.T) {
	repo := newMemoryProfileRepo()
	profiles := []*biz.Profile{
		{BindingRef: "binding-101", AccountKey: "account-101", ProfileRef: "profile-one", Region: "cn_gf01", PlayerID: "10001", IsDefault: true},
		{BindingRef: "binding-101", AccountKey: "account-101", ProfileRef: "profile-two", Region: "cn_gf01", PlayerID: "10002"},
	}
	for _, profile := range profiles {
		require.NoError(t, repo.Save(context.Background(), profile))
	}
	uc := NewProfileUsecase(repo)

	selected, err := uc.SetPrimaryProfileByRef(context.Background(), ScopeGuard{
		BindingRef: "binding-101",
		AccountKey: "account-101",
		ProfileRef: "profile-two",
	}, "account-101", "profile-two")
	require.NoError(t, err)
	require.Equal(t, "profile-two", selected.ProfileRef)
	require.True(t, selected.IsDefault)

	primary, err := uc.GetPrimaryProfile(context.Background(), "account-101")
	require.NoError(t, err)
	require.Equal(t, "profile-two", primary.ProfileRef)
}
