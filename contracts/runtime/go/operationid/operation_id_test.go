package operationid

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFingerprintContainsOnlyNonSensitiveOperationTuple(t *testing.T) {
	first := Fingerprint("replace", "binding-1", 4, 5)
	retry := Fingerprint("replace", "binding-1", 4, 5)
	changed := Fingerprint("replace", "binding-1", 5, 6)

	require.Equal(t, first, retry)
	require.NotEqual(t, first, changed)
	require.Equal(t, DeterministicID(first), DeterministicID(retry))
}

func TestNewIDCreatesOpaqueUniqueIntentIDs(t *testing.T) {
	first, err := NewID()
	require.NoError(t, err)
	second, err := NewID()
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.NotContains(t, first, "binding")
}

func TestAuthorizationFenceFingerprintIncludesEveryNonSensitiveMinimum(t *testing.T) {
	base := AuthorizationFenceFingerprint("fence", "binding-1", "consumer-1", 4, 2, 3, 5, 7)
	require.NotEqual(t, base, AuthorizationFenceFingerprint("fence", "binding-1", "consumer-2", 4, 2, 3, 5, 7))
	require.NotEqual(t, base, AuthorizationFenceFingerprint("fence", "binding-1", "consumer-1", 4, 3, 3, 5, 7))
	require.NotEqual(t, base, AuthorizationFenceFingerprint("fence", "binding-1", "consumer-1", 4, 2, 4, 5, 7))
	require.NotEqual(t, base, AuthorizationFenceFingerprint("fence", "binding-1", "consumer-1", 4, 2, 3, 6, 7))
	require.NotEqual(t, base, AuthorizationFenceFingerprint("fence", "binding-1", "consumer-1", 4, 2, 3, 5, 8))
}

func TestPrimaryProfileFingerprintIncludesProfileAndExpectedRevision(t *testing.T) {
	base := PrimaryProfileFingerprint("primary", "binding-1", "profile-1", 4, 7)
	require.Equal(t, base, PrimaryProfileFingerprint("primary", "binding-1", "profile-1", 4, 7))
	require.NotEqual(t, base, PrimaryProfileFingerprint("primary", "binding-1", "profile-2", 4, 7))
	require.NotEqual(t, base, PrimaryProfileFingerprint("primary", "binding-1", "profile-1", 4, 8))
}
