package usecase

import (
	"context"
	"strconv"
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
	artifact := artifactRepo.artifacts[bindingArtifactKey(101, authKeyArtifactType, "1008611")]
	require.NotNil(t, artifact)
	require.Equal(t, uint64(101), artifact.BindingID)
	require.Equal(t, first.AuthKey, artifact.ArtifactValue)
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

	credentialRepo.byPlatformAccountID["hoyo_10001"] = &biz.Credential{
		BindingID:         101,
		PlatformAccountID: "hoyo_10001",
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
	r.artifacts[bindingArtifactKey(artifact.BindingID, artifact.ArtifactType, artifact.ScopeKey)] = &clone
	return nil
}

func (r *memoryArtifactRepo) GetByBindingID(_ context.Context, bindingID uint64, artifactType, scopeKey string) (*biz.Artifact, error) {
	artifact := r.artifacts[bindingArtifactKey(bindingID, artifactType, scopeKey)]
	if artifact == nil || !artifact.ExpiresAt.After(time.Now()) {
		return nil, nil
	}
	clone := *artifact
	return &clone, nil
}

func (r *memoryArtifactRepo) Get(_ context.Context, platformAccountID, artifactType, scopeKey string) (*biz.Artifact, error) {
	artifact := r.artifacts[artifactKey(platformAccountID, artifactType, scopeKey)]
	if artifact == nil || !artifact.ExpiresAt.After(time.Now()) {
		return nil, nil
	}
	clone := *artifact
	return &clone, nil
}

func (r *memoryArtifactRepo) DeleteByPlatformAccountID(_ context.Context, platformAccountID string) error {
	for key, artifact := range r.artifacts {
		if artifact.PlatformAccountID == platformAccountID {
			delete(r.artifacts, key)
		}
	}
	return nil
}

func (r *memoryArtifactRepo) DeleteByBindingID(_ context.Context, bindingID uint64) error {
	for key, artifact := range r.artifacts {
		if artifact.BindingID == bindingID {
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

func artifactKey(platformAccountID, artifactType, scopeKey string) string {
	return platformAccountID + ":" + artifactType + ":" + scopeKey
}

func bindingArtifactKey(bindingID uint64, artifactType, scopeKey string) string {
	return strconv.FormatUint(bindingID, 10) + ":" + artifactType + ":" + scopeKey
}

var _ biz.ArtifactRepository = (*memoryArtifactRepo)(nil)
var _ platformmihomo.Client = (*authkeyTestClient)(nil)
