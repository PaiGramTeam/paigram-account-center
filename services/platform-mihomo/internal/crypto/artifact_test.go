package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArtifactCipherUsesVersionedResourceBoundAEAD(t *testing.T) {
	key := NewStaticKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	ciphertext, err := EncryptArtifact(key, "secret-authkey", "binding-1", "account-1", "authkey", "profile-1")
	require.NoError(t, err)
	require.NotContains(t, ciphertext, "secret-authkey")
	require.Contains(t, ciphertext, "v2.static.")

	plaintext, err := DecryptArtifact(key, ciphertext, "binding-1", "account-1", "authkey", "profile-1")
	require.NoError(t, err)
	require.Equal(t, "secret-authkey", plaintext)

	_, err = DecryptArtifact(key, ciphertext, "binding-2", "account-1", "authkey", "profile-1")
	require.ErrorIs(t, err, ErrInvalidArtifactCiphertext)
}
