package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

func TestValidateCredentialLazilyReencryptsWithActiveKey(t *testing.T) {
	keyring, rotate := newUsecaseKeyring(t)
	credentialRepo := newMemoryCredentialRepo()
	oldCiphertext, err := internalcrypto.EncryptString(keyring, `{"cookie_token":"old"}`)
	require.NoError(t, err)
	require.NoError(t, credentialRepo.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-rotate", AccountKey: "hoyo_10001", Region: "cn_gf01",
		CredentialBlob: oldCiphertext, CredentialVersion: "v2", Status: "active",
	}))
	rotate()
	usecase := NewStatusUsecase(
		credentialRepo, newMemoryProfileRepo(), successfulStatusClient{}, keyring,
		NewArtifactLifecycle(newMemoryArtifactRepo()),
	)

	_, err = usecase.ValidateCredential(context.Background(), "hoyo_10001")
	require.NoError(t, err)
	require.Contains(t, credentialRepo.byAccountKey["hoyo_10001"].CredentialBlob, ".new.")
}

func TestValidateCredentialRotationPersistsAttentionAndInvalidatesArtifacts(t *testing.T) {
	keyring, rotate := newUsecaseKeyring(t)
	credentialRepo := newMemoryCredentialRepo()
	artifactRepo := newMemoryArtifactRepo()
	oldCiphertext, err := internalcrypto.EncryptString(keyring, `{"cookie_token":"expired"}`)
	require.NoError(t, err)
	require.NoError(t, credentialRepo.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-101", AccountKey: "hoyo_10001", Generation: 1, Region: "cn_gf01",
		CredentialBlob: oldCiphertext, CredentialVersion: "v2", Status: "active",
	}))
	putStatusTestArtifact(t, artifactRepo)
	rotate()
	usecase := NewStatusUsecase(
		credentialRepo, newMemoryProfileRepo(),
		failingStatusClient{err: &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorExpiredCredential}},
		keyring, statusTestArtifactLifecycle(artifactRepo),
	)

	output, err := usecase.ValidateCredential(context.Background(), "hoyo_10001")
	require.NoError(t, err)
	require.Equal(t, CredentialStatusExpired, output.Status)
	stored := credentialRepo.byAccountKey["hoyo_10001"]
	require.Equal(t, "expired", stored.Status)
	require.Contains(t, stored.CredentialBlob, ".new.")
	require.Empty(t, artifactRepo.artifacts)
}

func TestBackgroundReencryptionRaceStillPersistsCredentialAttention(t *testing.T) {
	keyring, rotate := newUsecaseKeyring(t)
	credentialRepo := newMemoryCredentialRepo()
	const plaintext = `{"cookie_token":"expired"}`
	oldCiphertext, err := internalcrypto.EncryptString(keyring, plaintext)
	require.NoError(t, err)
	require.NoError(t, credentialRepo.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-101", AccountKey: "hoyo_10001", Generation: 1, Region: "cn_gf01",
		CredentialBlob: oldCiphertext, CredentialVersion: "v2", Status: "active",
	}))
	rotate()
	client := callbackStatusClient{callback: func() {
		current := credentialRepo.byAccountKey["hoyo_10001"]
		reencrypted, encryptErr := internalcrypto.EncryptString(keyring, plaintext)
		require.NoError(t, encryptErr)
		copy := *current
		copy.CredentialBlob = reencrypted
		require.NoError(t, credentialRepo.Save(context.Background(), &copy))
	}}
	usecase := NewStatusUsecase(
		credentialRepo, newMemoryProfileRepo(), client, keyring,
		NewArtifactLifecycle(newMemoryArtifactRepo()),
	)

	output, err := usecase.ValidateCredential(context.Background(), "hoyo_10001")
	require.NoError(t, err)
	require.Equal(t, CredentialStatusExpired, output.Status)
	require.Equal(t, "expired", credentialRepo.byAccountKey["hoyo_10001"].Status)
	require.Contains(t, credentialRepo.byAccountKey["hoyo_10001"].CredentialBlob, ".new.")
}

type callbackStatusClient struct {
	callback func()
}

func (c callbackStatusClient) ValidateAndDiscover(context.Context, string, string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	c.callback()
	return "", "", nil, &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorExpiredCredential}
}

func (callbackStatusClient) RefreshCredential(context.Context, string, string) (platformmihomo.RefreshResult, error) {
	return platformmihomo.RefreshResult{}, &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorExpiredCredential}
}

func (callbackStatusClient) IssueAuthKey(context.Context, string, string) (string, int64, error) {
	return "", 0, errors.New("not implemented")
}

func TestCachedAuthKeyLazilyReencryptsCredentialAndArtifact(t *testing.T) {
	keyring, rotate := newUsecaseKeyring(t)
	credentialRepo := newMemoryCredentialRepo()
	artifactRepo := newMemoryArtifactRepo()
	credentialCiphertext, err := internalcrypto.EncryptString(keyring, `{"cookie_token":"old"}`)
	require.NoError(t, err)
	require.NoError(t, credentialRepo.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-rotate", AccountKey: "hoyo_10001", Generation: 1,
		CredentialBlob: credentialCiphertext, CredentialVersion: "v2", Status: "active",
	}))
	artifactCiphertext, err := internalcrypto.EncryptArtifact(keyring, "cached-authkey", "binding-rotate", "hoyo_10001", authKeyArtifactType, "player")
	require.NoError(t, err)
	require.NoError(t, artifactRepo.Put(context.Background(), &biz.Artifact{
		BindingRef: "binding-rotate", AccountKey: "hoyo_10001", ArtifactType: authKeyArtifactType,
		ArtifactValue: artifactCiphertext, ScopeKey: "player", ExpiresAt: time.Now().Add(time.Minute),
	}))
	rotate()
	usecase := NewAuthkeyUsecase(
		credentialRepo, artifactRepo, NewArtifactLifecycle(artifactRepo), &authkeyTestClient{}, keyring,
	)

	output, err := usecase.GetAuthKey(context.Background(), "hoyo_10001", "player")
	require.NoError(t, err)
	require.Equal(t, "cached-authkey", output.AuthKey)
	require.Contains(t, credentialRepo.byAccountKey["hoyo_10001"].CredentialBlob, ".new.")
	require.Contains(t, artifactRepo.artifacts[bindingArtifactKey("binding-rotate", authKeyArtifactType, "player")].ArtifactValue, ".new.")
}

func newUsecaseKeyring(t *testing.T) (*internalcrypto.FileKeyring, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "encryption-keyring.json")
	oldKey := []byte("0123456789abcdef0123456789abcdef")
	newKey := []byte("abcdef0123456789abcdef0123456789")
	writeUsecaseKeyring(t, path, "old", map[string][]byte{"old": oldKey})
	keyring, err := internalcrypto.NewFileKeyring(path)
	require.NoError(t, err)
	return keyring, func() {
		writeUsecaseKeyring(t, path, "new", map[string][]byte{"old": oldKey, "new": newKey})
	}
}

func writeUsecaseKeyring(t *testing.T, path, activeKeyID string, keys map[string][]byte) {
	t.Helper()
	entries := make([]internalcrypto.KeyringEntry, 0, len(keys))
	for keyID, key := range keys {
		entries = append(entries, internalcrypto.KeyringEntry{KeyID: keyID, KeyBase64: base64.RawStdEncoding.EncodeToString(key)})
	}
	raw, err := json.Marshal(internalcrypto.KeyringFile{ActiveKeyID: activeKeyID, Keys: entries})
	require.NoError(t, err)
	temporary := path + ".new"
	require.NoError(t, os.WriteFile(temporary, raw, 0o600))
	require.NoError(t, os.Rename(temporary, path))
}
