package crypto

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileKeyringRotatesWithReadOverlapAndRetirement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "encryption-keyring.json")
	oldKey := []byte("0123456789abcdef0123456789abcdef")
	newKey := []byte("abcdef0123456789abcdef0123456789")
	writeEncryptionKeyring(t, path, "old", map[string][]byte{"old": oldKey})
	keyring, err := NewFileKeyring(path)
	require.NoError(t, err)
	oldCiphertext, err := EncryptString(keyring, "old-secret")
	require.NoError(t, err)

	writeEncryptionKeyring(t, path, "new", map[string][]byte{"old": oldKey, "new": newKey})
	newCiphertext, err := EncryptString(keyring, "new-secret")
	require.NoError(t, err)
	require.Contains(t, newCiphertext, ".new.")
	oldPlaintext, err := DecryptString(keyring, oldCiphertext)
	require.NoError(t, err)
	require.Equal(t, "old-secret", oldPlaintext)

	writeEncryptionKeyring(t, path, "new", map[string][]byte{"new": newKey})
	_, err = DecryptString(keyring, oldCiphertext)
	require.ErrorIs(t, err, ErrInvalidKeyring)
	newPlaintext, err := DecryptString(keyring, newCiphertext)
	require.NoError(t, err)
	require.Equal(t, "new-secret", newPlaintext)
}

func TestArtifactEnvelopeBindsKeyIDAndResourceContext(t *testing.T) {
	provider := NewStaticKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	ciphertext, err := EncryptArtifact(provider, "auth-key", "binding", "account", "authkey", "profile")
	require.NoError(t, err)
	require.Contains(t, ciphertext, "v2.static.")
	plaintext, err := DecryptArtifact(provider, ciphertext, "binding", "account", "authkey", "profile")
	require.NoError(t, err)
	require.Equal(t, "auth-key", plaintext)
	_, err = DecryptArtifact(provider, ciphertext, "other-binding", "account", "authkey", "profile")
	require.ErrorIs(t, err, ErrInvalidArtifactCiphertext)
}

func TestFileKeyringRejectsKeyIDsThatBreakEnvelopeParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "encryption-keyring.json")
	key := []byte("0123456789abcdef0123456789abcdef")
	for _, keyID := range []string{"key.2026", "key 2026", "", "键", strings.Repeat("a", 65)} {
		t.Run(keyID, func(t *testing.T) {
			writeEncryptionKeyring(t, path, keyID, map[string][]byte{keyID: key})
			_, err := NewFileKeyring(path)
			require.ErrorIs(t, err, ErrInvalidKeyring)
		})
	}
}

func writeEncryptionKeyring(t *testing.T, path, activeKeyID string, keys map[string][]byte) {
	t.Helper()
	entries := make([]KeyringEntry, 0, len(keys))
	for keyID, key := range keys {
		entries = append(entries, KeyringEntry{KeyID: keyID, KeyBase64: base64.RawStdEncoding.EncodeToString(key)})
	}
	raw, err := json.Marshal(KeyringFile{ActiveKeyID: activeKeyID, Keys: entries})
	require.NoError(t, err)
	temporary := path + ".new"
	require.NoError(t, os.WriteFile(temporary, raw, 0o600))
	require.NoError(t, os.Rename(temporary, path))
}
