package usecase

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
)

type memoryCredentialReencryptionRepository struct {
	credentials map[string]*biz.Credential
}

func (r *memoryCredentialReencryptionRepository) ListCredentialReencryptionBatch(_ context.Context, after string, limit int) ([]*biz.Credential, error) {
	refs := make([]string, 0, len(r.credentials))
	for ref := range r.credentials {
		if ref > after {
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	if len(refs) > limit {
		refs = refs[:limit]
	}
	result := make([]*biz.Credential, 0, len(refs))
	for _, ref := range refs {
		clone := *r.credentials[ref]
		result = append(result, &clone)
	}
	return result, nil
}

func (r *memoryCredentialReencryptionRepository) ReencryptCredentialBlob(_ context.Context, bindingRef, expected, replacement string) (bool, error) {
	credential := r.credentials[bindingRef]
	if credential == nil || credential.CredentialBlob != expected {
		return false, nil
	}
	credential.CredentialBlob = replacement
	credential.CredentialVersion = "v2"
	return true, nil
}

func TestCredentialReencryptionMigratesPersistentCredentialsBeforeKeyRetirement(t *testing.T) {
	keyring, rotate := newUsecaseKeyring(t)
	oldCiphertext, err := internalcrypto.EncryptString(keyring, `{"cookie_token":"old"}`)
	require.NoError(t, err)
	repository := &memoryCredentialReencryptionRepository{credentials: map[string]*biz.Credential{
		"binding-1": {BindingRef: "binding-1", CredentialBlob: oldCiphertext, CredentialVersion: "v2"},
	}}
	rotate()
	usecase := NewCredentialReencryptionUsecase(repository, keyring)

	updated, err := usecase.ReencryptAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), updated)
	require.Contains(t, repository.credentials["binding-1"].CredentialBlob, ".new.")
	plaintext, err := internalcrypto.DecryptString(keyring, repository.credentials["binding-1"].CredentialBlob)
	require.NoError(t, err)
	require.JSONEq(t, `{"cookie_token":"old"}`, plaintext)
}

func TestCredentialReencryptionDoesNotOverwriteConcurrentReplacement(t *testing.T) {
	keyring, rotate := newUsecaseKeyring(t)
	oldCiphertext, err := internalcrypto.EncryptString(keyring, `{"cookie_token":"old"}`)
	require.NoError(t, err)
	repository := &memoryCredentialReencryptionRepository{credentials: map[string]*biz.Credential{
		"binding-1": {BindingRef: "binding-1", CredentialBlob: oldCiphertext},
	}}
	rotate()
	credentials, err := repository.ListCredentialReencryptionBatch(context.Background(), "", 100)
	require.NoError(t, err)
	repository.credentials["binding-1"].CredentialBlob, err = internalcrypto.EncryptString(keyring, `{"cookie_token":"replacement"}`)
	require.NoError(t, err)
	replaced, err := repository.ReencryptCredentialBlob(context.Background(), "binding-1", credentials[0].CredentialBlob, "stale")
	require.NoError(t, err)
	require.False(t, replaced)
}
