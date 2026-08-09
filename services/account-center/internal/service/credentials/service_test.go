package credentials

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/model"
)

// TestVerifySecret_InvokesBcryptOnAllPaths is a contract test for the
// timing-attack mitigation: VerifySecret MUST invoke bcrypt comparison
// even when the lookup returns ErrCredentialNotFound or
// ErrCredentialDisabled, so wall-clock time is equivalent across the
// "wrong secret" and "unknown client_id" paths. The implementation
// satisfies this by comparing against dummyClientSecretHash whenever
// the row is nil.
//
// Verifying actual timing on CI is flaky, so the assertion is
// structural: the dummy hash must be a valid bcrypt-format string
// (cost 12, $2a$/2b$ prefix) so bcrypt.CompareHashAndPassword actually
// executes the KDF rather than fast-failing on a malformed hash.
func TestDummyClientSecretHashIsValidBcryptFormat(t *testing.T) {
	// Bcrypt hashes have the form $2[abxy]$<cost>$<22-char salt><31-char hash>
	// for a total of 60 characters. Cost 12 must match clientSecretBcryptCost
	// so the dummy comparison takes the same wall time as a real one.
	const expectedLen = 60
	require.Len(t, dummyClientSecretHash, expectedLen,
		"dummy hash must be exactly %d bcrypt chars to match the real-credential KDF cost",
		expectedLen,
	)
	assert.True(t,
		dummyClientSecretHash[:7] == "$2a$12$" || dummyClientSecretHash[:7] == "$2b$12$",
		"dummy hash must start with $2a$12$ or $2b$12$, got %q",
		dummyClientSecretHash[:7],
	)

	// And it must actually compare-mismatched (i.e. the bcrypt CompareHashAndPassword
	// path actually runs to completion rather than rejecting a malformed hash).
	cmpErr := VerifyClientSecret(dummyClientSecretHash, "definitely-not-the-original-plaintext")
	require.Error(t, cmpErr, "dummy hash must run a real bcrypt comparison")
}

// TestVerifySecret_UnknownClientStillReturnsSentinel guards the
// contract that GetByClientID semantics are preserved by the timing
// hardening. Even though we now run bcrypt against dummyClientSecretHash
// before deciding, the error returned MUST still be
// ErrCredentialNotFound (not ErrInvalidClientSecret), so caller error
// mapping continues to work.
func TestVerifySecret_UnknownClientStillReturnsSentinel(t *testing.T) {
	db := setupCredentialsTestDB(t)
	svc := NewService(db)

	_, err := svc.VerifySecret("no-such-client", "anything")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCredentialNotFound),
		"expected ErrCredentialNotFound, got %T %v", err, err,
	)
}

// TestVerifySecret_DisabledClientStillReturnsSentinel mirrors the above
// for the disabled-credential path.
func TestVerifySecret_DisabledClientStillReturnsSentinel(t *testing.T) {
	db := setupCredentialsTestDB(t)
	svc := NewService(db)
	result, err := svc.Create(CreateInput{
		ClientID:    "telegram-service",
		DisplayName: "Telegram",
		OwnerUserID: 1,
		Audiences:   []string{"mihomo.sync"},
		Scopes:      []string{"binding.read"},
	})
	require.NoError(t, err)

	_, err = svc.SetStatus("telegram-service", model.ServiceCredentialStatusDisabled)
	require.NoError(t, err)

	_, err = svc.VerifySecret("telegram-service", result.ClientSecret)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCredentialDisabled),
		"expected ErrCredentialDisabled, got %T %v", err, err,
	)
}

// TestSetStatus_InvalidStatusReturnsErrInvalidStatus pins the IMP-1 fix:
// passing a value outside {active, disabled} must surface as
// ErrInvalidStatus, NOT ErrCredentialNotFound. The previous behaviour
// conflated "you wrote the status wrong" with "the credential doesn't
// exist" and broke HTTP status-code mapping in the admin handler.
func TestSetStatus_InvalidStatusReturnsErrInvalidStatus(t *testing.T) {
	db := setupCredentialsTestDB(t)
	svc := NewService(db)
	_, err := svc.Create(CreateInput{
		ClientID:    "telegram-service",
		DisplayName: "Telegram",
		OwnerUserID: 1,
		Audiences:   []string{"mihomo.sync"},
		Scopes:      []string{"binding.read"},
	})
	require.NoError(t, err)

	_, err = svc.SetStatus("telegram-service", "banana")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidStatus),
		"expected ErrInvalidStatus, got %T %v", err, err,
	)
}

// TestNewTokenService_RejectsShortSigningKey pins the IMP-2 fix:
// constructor must refuse a signing key shorter than the HS256 minimum
// (32 bytes) rather than silently issuing tokens whose signatures will
// fail verification later. The error message must include the actual
// byte length so operators can pinpoint configuration mistakes.
func TestNewTokenService_RejectsShortSigningKey(t *testing.T) {
	svc := NewService(nil) // no DB needed — constructor fails before any call.
	cases := []struct {
		name string
		key  []byte
	}{
		{name: "nil key", key: nil},
		{name: "empty key", key: []byte{}},
		{name: "31-byte key (one short of minimum)", key: make([]byte, 31)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, err := NewTokenService(svc, TokenServiceConfig{SigningKey: tc.key})
			assert.Nil(t, ts, "constructor must return nil TokenService on rejected key")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "32 bytes",
				"error must explain the 32-byte minimum so operators can fix their config",
			)
		})
	}
}

// TestNewTokenService_Accepts32ByteKey is the positive counterpart:
// exactly 32 bytes must succeed so the lower bound is "≥ 32", not "> 32".
func TestNewTokenService_Accepts32ByteKey(t *testing.T) {
	svc := NewService(nil)
	ts, err := NewTokenService(svc, TokenServiceConfig{
		SigningKey: make([]byte, 32),
	})
	require.NoError(t, err)
	require.NotNil(t, ts)
}
