package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

func TestGetAuthKeyReturnsCachedArtifactWhenPresent(t *testing.T) {
	uc, artifactRepo, client := newAuthkeyUsecaseForTest(t)

	first, err := uc.GetAuthKey(context.Background(), "hoyo_10001", "1008611")
	require.NoError(t, err)
	require.Equal(t, "issued-authkey-1", first.AuthKey)
	require.Equal(t, 1, client.issueAuthKeyCalls)

	second, err := uc.GetAuthKey(context.Background(), "hoyo_10001", "1008611")
	require.NoError(t, err)
	require.Equal(t, first.AuthKey, second.AuthKey)
	require.Equal(t, first.ExpiresAt, second.ExpiresAt)
	require.Equal(t, 1, client.issueAuthKeyCalls)
	artifact := artifactRepo.artifacts[bindingArtifactKey("binding-101", authKeyArtifactType, "1008611")]
	require.NotNil(t, artifact)
	require.Equal(t, "binding-101", artifact.BindingRef)
	require.NotEqual(t, first.AuthKey, artifact.ArtifactValue)
	decrypted, err := internalcrypto.DecryptArtifact(testEncryptionKey, artifact.ArtifactValue, artifact.BindingRef, artifact.AccountKey, artifact.ArtifactType, artifact.ScopeKey)
	require.NoError(t, err)
	require.Equal(t, first.AuthKey, decrypted)
	require.WithinDuration(t, first.ExpiresAt, artifact.ExpiresAt, time.Second)
}

func TestGetAuthKeyReturnsErrorWhenCredentialIsMissing(t *testing.T) {
	uc, _, _ := newAuthkeyUsecaseForTest(t)

	_, err := uc.GetAuthKey(context.Background(), "hoyo_missing", "1008611")
	require.Error(t, err)
	require.ErrorContains(t, err, "credential not found")
}

func newAuthkeyUsecaseForTest(t *testing.T) (*AuthkeyUsecase, *memoryArtifactRepo, *authkeyTestClient) {
	t.Helper()

	credentialRepo := newMemoryCredentialRepo()
	artifactRepo := newMemoryArtifactRepo()
	client := &authkeyTestClient{}

	encryptedBlob, err := internalcrypto.EncryptString(testEncryptionKey, `{"account_id":"10001","cookie_token":"abc"}`)
	require.NoError(t, err)

	credentialRepo.byAccountKey["hoyo_10001"] = &biz.Credential{
		BindingRef:        "binding-101",
		AccountKey:        "hoyo_10001",
		Platform:          "mihomo",
		AccountID:         "10001",
		Region:            "cn_gf01",
		CredentialBlob:    encryptedBlob,
		CredentialVersion: "v1",
		Status:            "active",
	}

	return NewAuthkeyUsecase(credentialRepo, artifactRepo, client, testEncryptionKey), artifactRepo, client
}

type memoryArtifactRepo struct {
	artifacts map[string]*biz.Artifact
}

func newMemoryArtifactRepo() *memoryArtifactRepo {
	return &memoryArtifactRepo{artifacts: make(map[string]*biz.Artifact)}
}

func (r *memoryArtifactRepo) Put(_ context.Context, artifact *biz.Artifact) error {
	clone := *artifact
	r.artifacts[bindingArtifactKey(artifact.BindingRef, artifact.ArtifactType, artifact.ScopeKey)] = &clone
	return nil
}

func (r *memoryArtifactRepo) GetByBindingRef(_ context.Context, bindingRef string, artifactType, scopeKey string) (*biz.Artifact, error) {
	artifact := r.artifacts[bindingArtifactKey(bindingRef, artifactType, scopeKey)]
	if artifact == nil || !artifact.ExpiresAt.After(time.Now()) {
		return nil, nil
	}
	clone := *artifact
	return &clone, nil
}

func (r *memoryArtifactRepo) Get(_ context.Context, accountKey, artifactType, scopeKey string) (*biz.Artifact, error) {
	artifact := r.artifacts[artifactKey(accountKey, artifactType, scopeKey)]
	if artifact == nil || !artifact.ExpiresAt.After(time.Now()) {
		return nil, nil
	}
	clone := *artifact
	return &clone, nil
}

func (r *memoryArtifactRepo) DeleteByAccountKey(_ context.Context, accountKey string) error {
	for key, artifact := range r.artifacts {
		if artifact.AccountKey == accountKey {
			delete(r.artifacts, key)
		}
	}
	return nil
}

func (r *memoryArtifactRepo) DeleteByBindingRef(_ context.Context, bindingRef string) error {
	for key, artifact := range r.artifacts {
		if artifact.BindingRef == bindingRef {
			delete(r.artifacts, key)
		}
	}
	return nil
}

type authkeyTestClient struct {
	issueAuthKeyCalls int
}

func (c *authkeyTestClient) ValidateAndDiscover(_ context.Context, _ string, _ string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	return "", "", nil, nil
}

func (c *authkeyTestClient) IssueAuthKey(_ context.Context, cookieBundleJSON string, playerID string) (string, int64, error) {
	c.issueAuthKeyCalls++
	if cookieBundleJSON == "" {
		return "", 0, nil
	}
	return "issued-authkey-1", 300, nil
}

func artifactKey(accountKey, artifactType, scopeKey string) string {
	return accountKey + ":" + artifactType + ":" + scopeKey
}

func bindingArtifactKey(bindingRef string, artifactType, scopeKey string) string {
	return bindingRef + ":" + artifactType + ":" + scopeKey
}

var _ biz.ArtifactRepository = (*memoryArtifactRepo)(nil)
var _ platformmihomo.Client = (*authkeyTestClient)(nil)
