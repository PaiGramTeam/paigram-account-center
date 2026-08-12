//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/usecase"
)

func TestCredentialKeyRotationReencryptsPersistentPostgreSQLRecords(t *testing.T) {
	stack := newIntegrationStack(t)
	keyringPath := filepath.Join(t.TempDir(), "credential-keyring.json")
	oldKey := []byte("0123456789abcdef0123456789abcdef")
	newKey := []byte("abcdef0123456789abcdef0123456789")
	writeCredentialKeyring(t, keyringPath, "old", map[string][]byte{"old": oldKey})
	keyring, err := internalcrypto.NewFileKeyring(keyringPath)
	require.NoError(t, err)

	const plaintext = `{"account_id":"10001","cookie_token":"rotation-secret"}`
	oldEnvelope, err := internalcrypto.EncryptString(keyring, plaintext)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(oldEnvelope, "v2.old."))

	repository := data.NewCredentialRepo(stack.DB)
	require.NoError(t, repository.Create(context.Background(), &biz.Credential{
		BindingRef:        "bind_key_rotation",
		AccountKey:        "acct_key_rotation",
		Generation:        1,
		Platform:          "mihomo",
		AccountID:         "10001",
		Region:            "cn_gf01",
		CredentialBlob:    oldEnvelope,
		CredentialVersion: "v2",
		Status:            "active",
	}))

	writeCredentialKeyring(t, keyringPath, "new", map[string][]byte{"old": oldKey, "new": newKey})
	updated, err := usecase.NewCredentialReencryptionUsecase(repository, keyring).ReencryptAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), updated)

	var storedEnvelope string
	require.NoError(t, stack.DB.Raw(
		"SELECT credential_blob FROM credential_records WHERE binding_ref = ?",
		"bind_key_rotation",
	).Scan(&storedEnvelope).Error)
	require.True(t, strings.HasPrefix(storedEnvelope, "v2.new."))
	decrypted, err := internalcrypto.DecryptString(keyring, storedEnvelope)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)

	writeCredentialKeyring(t, keyringPath, "new", map[string][]byte{"new": newKey})
	decrypted, err = internalcrypto.DecryptString(keyring, storedEnvelope)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
	_, err = internalcrypto.DecryptString(keyring, oldEnvelope)
	require.Error(t, err)
}

func writeCredentialKeyring(t *testing.T, path string, activeKeyID string, keys map[string][]byte) {
	t.Helper()
	entries := make([]internalcrypto.KeyringEntry, 0, len(keys))
	for keyID, key := range keys {
		entries = append(entries, internalcrypto.KeyringEntry{
			KeyID:     keyID,
			KeyBase64: base64.RawStdEncoding.EncodeToString(key),
		})
	}
	payload, err := json.Marshal(internalcrypto.KeyringFile{ActiveKeyID: activeKeyID, Keys: entries})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, payload, 0o600))
}
