package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
)

func TestGuardRejectsForeignProfileRef(t *testing.T) {
	guard := ScopeGuard{
		AllowedActions: map[string]struct{}{"mihomo.profile.read": {}},
		BindingRef:     "binding-42",
		AccountKey:     "account-42",
		ProfileRef:     "profile-1001",
	}

	err := guard.RequireProfile("binding-42", "profile-2002")
	require.ErrorIs(t, err, ErrProfileScopeDenied)
}

func TestGuardRequiresExactAccountKeyClaim(t *testing.T) {
	guard := ScopeGuard{BindingRef: "binding-42", AccountKey: "account-42"}

	require.NoError(t, guard.RequireAccountKey("account-42"))
	require.ErrorIs(t, guard.RequireAccountKey("account-foreign"), ErrBindingScopeDenied)
}

func TestConfirmPrimaryProfileWithScopeRejectsForeignProfile(t *testing.T) {
	harness := newBindUsecaseForTest()
	resp, err := harness.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
	})
	require.NoError(t, err)

	_, err = harness.profileUsecase.ConfirmPrimaryProfileWithScope(context.Background(), ScopeGuard{
		BindingRef: "binding-42",
		AccountKey: resp.AccountKey,
		ProfileRef: "profile-foreign",
	}, resp.AccountKey, "1008611")
	require.ErrorIs(t, err, ErrProfileScopeDenied)
}

func TestConfirmPrimaryProfileWithScopeRejectsProfileScopedTicket(t *testing.T) {
	harness := newBindUsecaseForTest()
	resp, err := harness.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Profiles)

	_, err = harness.profileUsecase.ConfirmPrimaryProfileWithScope(context.Background(), ScopeGuard{
		BindingRef: "binding-42",
		AccountKey: resp.AccountKey,
		ProfileRef: resp.Profiles[0].ProfileRef,
	}, resp.AccountKey, "1008611")
	require.ErrorIs(t, err, ErrProfileScopeDenied)
}

func TestScopeGuardRequiresExactProfileClaimForProfileMutation(t *testing.T) {
	require.ErrorIs(t, (ScopeGuard{BindingRef: "binding-1"}).RequireExactProfile("profile-1"), ErrProfileScopeDenied)
	require.ErrorIs(t, (ScopeGuard{BindingRef: "binding-1", ProfileRef: "profile-2"}).RequireExactProfile("profile-1"), ErrProfileScopeDenied)
	require.NoError(t, (ScopeGuard{BindingRef: "binding-1", ProfileRef: "profile-1"}).RequireExactProfile("profile-1"))
}

var _ biz.ProfileRepository = (*memoryProfileRepo)(nil)
