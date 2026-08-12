package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplaceCredentialKeepsProfileRevisionMonotonic(t *testing.T) {
	harness := newBindUsecaseForTest()
	input := BindCredentialInput{
		BindingRef:       "binding-42",
		Generation:       1,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	}
	first, err := harness.BindCredential(context.Background(), input)
	require.NoError(t, err)
	_, err = harness.credentialRepo.AdvanceProfileRevision(context.Background(), input.BindingRef, first.AccountKey, 1, 1)
	require.NoError(t, err)
	_, err = harness.credentialRepo.AdvanceProfileRevision(context.Background(), input.BindingRef, first.AccountKey, 1, 2)
	require.NoError(t, err)

	input.Generation = 2
	input.CookieBundleJSON = `{"account_id":"10001","cookie_token":"rotated"}`
	_, err = harness.BindCredential(context.Background(), input)
	require.NoError(t, err)

	credential, err := harness.credentialRepo.GetByBindingRef(context.Background(), input.BindingRef)
	require.NoError(t, err)
	require.Equal(t, uint64(4), credential.ProfileRevision)
	require.Equal(t, uint64(4), credential.ProfileObservedRevision)
}
