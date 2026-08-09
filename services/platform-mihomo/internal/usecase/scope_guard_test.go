package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
)

func TestGuardProfileAccessRejectsForeignProfile(t *testing.T) {
	guard := ScopeGuard{AllowedActions: map[string]struct{}{"mihomo.profile.read": {}}, BindingID: 42, ProfileID: 1001}

	err := guard.RequireProfile(42, 2002)
	require.ErrorIs(t, err, ErrProfileScopeDenied)
}

func TestGuardAcceptsLegacyPlatformAccountID(t *testing.T) {
	guard := ScopeGuard{AllowedActions: map[string]struct{}{"mihomo.profile.read": {}}, BindingID: 42}

	err := guard.RequirePlatformAccountID("hoyo_ref_42_10001")
	require.NoError(t, err)
}

func TestConfirmPrimaryProfileWithScopeRejectsForeignProfile(t *testing.T) {
	harness := newBindUsecaseForTest()

	resp, err := harness.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
	})
	require.NoError(t, err)

	profiles := harness.profileRepo.byPlatformAccountID[resp.PlatformAccountID]
	require.Len(t, profiles, 1)
	profiles[0].ID = 2002
	harness.profileRepo.byPlatformAccountID[resp.PlatformAccountID][0].ID = 2002
	harness.profileRepo.byBindingID[42][0].ID = 2002

	_, err = harness.profileUsecase.ConfirmPrimaryProfileWithScope(context.Background(), ScopeGuard{
		AllowedActions: map[string]struct{}{"mihomo.profile.write": {}},
		BindingID:      42,
		ProfileID:      1001,
	}, resp.PlatformAccountID, "1008611")
	require.ErrorIs(t, err, ErrProfileScopeDenied)
}

func TestConfirmPrimaryProfileWithScopeRejectsProfileScopedTicket(t *testing.T) {
	harness := newBindUsecaseForTest()

	resp, err := harness.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
	})
	require.NoError(t, err)

	profiles := harness.profileRepo.byPlatformAccountID[resp.PlatformAccountID]
	require.Len(t, profiles, 1)
	profiles[0].ID = 1001
	harness.profileRepo.byPlatformAccountID[resp.PlatformAccountID][0].ID = 1001
	harness.profileRepo.byBindingID[42][0].ID = 1001

	_, err = harness.profileUsecase.ConfirmPrimaryProfileWithScope(context.Background(), ScopeGuard{
		AllowedActions: map[string]struct{}{"mihomo.profile.write": {}},
		BindingID:      42,
		ProfileID:      1001,
	}, resp.PlatformAccountID, "1008611")
	require.ErrorIs(t, err, ErrProfileScopeDenied)
}

var _ biz.ProfileRepository = (*memoryProfileRepo)(nil)
