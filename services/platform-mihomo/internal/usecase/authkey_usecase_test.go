package usecase

import (
	"context"
	"errors"
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

func TestGetAuthKeyRejectsArtifactForInvalidCredential(t *testing.T) {
	uc, artifactRepo, client := newAuthkeyUsecaseForTest(t)
	credential := uc.credentialRepo.(*memoryCredentialRepo).byAccountKey["hoyo_10001"]
	credential.Status = "expired"
	require.NoError(t, uc.credentialRepo.Save(context.Background(), credential))
	ciphertext, err := internalcrypto.EncryptArtifact(testEncryptionKey, "issued-authkey", "binding-101", "hoyo_10001", authKeyArtifactType, "1008611")
	require.NoError(t, err)
	require.NoError(t, artifactRepo.Put(context.Background(), &biz.Artifact{
		BindingRef: "binding-101", AccountKey: "hoyo_10001", ArtifactType: authKeyArtifactType,
		ArtifactValue: ciphertext, ScopeKey: "1008611", ExpiresAt: time.Now().Add(time.Hour),
	}))

	_, err = uc.GetAuthKey(context.Background(), "hoyo_10001", "1008611")
	require.ErrorIs(t, err, ErrCredentialRequiresAttention)
	require.Zero(t, client.issueAuthKeyCalls)
}

func TestGetAuthKeyInvalidUpstreamRevokesOtherProfileArtifacts(t *testing.T) {
	uc, artifactRepo, client := newAuthkeyUsecaseForTest(t)
	ciphertext, err := internalcrypto.EncryptArtifact(testEncryptionKey, "other-profile-key", "binding-101", "hoyo_10001", authKeyArtifactType, "other-player")
	require.NoError(t, err)
	require.NoError(t, artifactRepo.Put(context.Background(), &biz.Artifact{
		BindingRef: "binding-101", AccountKey: "hoyo_10001", ArtifactType: authKeyArtifactType,
		ArtifactValue: ciphertext, ScopeKey: "other-player", ExpiresAt: time.Now().Add(time.Hour),
	}))
	client.issueErr = &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorExpiredCredential}

	_, err = uc.GetAuthKey(context.Background(), "hoyo_10001", "new-player")
	require.Error(t, err)
	require.True(t, platformmihomo.IsErrorKind(err, platformmihomo.ErrorExpiredCredential))
	require.Equal(t, "expired", uc.credentialRepo.(*memoryCredentialRepo).byAccountKey["hoyo_10001"].Status)
	require.Empty(t, artifactRepo.artifacts)
	require.Equal(t, []string{"other-profile-key"}, client.revoked)
}

func TestGetAuthKeyCommitFailureLeavesDurableRevocationIntent(t *testing.T) {
	uc, artifactRepo, client := newAuthkeyUsecaseForTest(t)
	credentialRepo := uc.credentialRepo.(*memoryCredentialRepo)
	credentialRepo.transactionCommitErr = errors.New("commit failed")
	client.revokeErr = errors.New("revoke unavailable")

	_, err := uc.GetAuthKey(context.Background(), "hoyo_10001", "1008611")
	require.ErrorContains(t, err, "commit failed")
	require.Empty(t, artifactRepo.artifacts)
	require.Len(t, artifactRepo.revocationIntents, 1)
	for _, intent := range artifactRepo.revocationIntents {
		require.NotEqual(t, "issued-authkey-1", intent.ArtifactValue)
		decrypted, decryptErr := internalcrypto.DecryptArtifact(
			testEncryptionKey,
			intent.ArtifactValue,
			intent.BindingRef,
			intent.AccountKey,
			intent.ArtifactType,
			intent.ScopeKey,
		)
		require.NoError(t, decryptErr)
		require.Equal(t, "issued-authkey-1", decrypted)
	}
	require.Equal(t, 1, client.revokeAttempts)

	credentialRepo.transactionCommitErr = nil
	client.revokeErr = nil
	require.NoError(t, uc.artifacts.RetryPending(context.Background()))
	require.Empty(t, artifactRepo.revocationIntents)
	require.Equal(t, 2, client.revokeAttempts)
	require.Equal(t, []string{"issued-authkey-1"}, client.revoked)
}

func TestGetAuthKeyInvalidTTLUsesDurableRevocationIntent(t *testing.T) {
	uc, artifactRepo, client := newAuthkeyUsecaseForTest(t)
	client.expiresInSeconds = int64(maximumAuthKeyTTL/time.Second) + 1
	client.revokeErr = errors.New("revoke unavailable")

	_, err := uc.GetAuthKey(context.Background(), "hoyo_10001", "1008611")
	require.Error(t, err)
	require.True(t, platformmihomo.IsErrorKind(err, platformmihomo.ErrorInvalidResponse))
	require.Len(t, artifactRepo.revocationIntents, 1)
	require.Empty(t, artifactRepo.artifacts)
}

func newAuthkeyUsecaseForTest(t *testing.T) (*AuthkeyUsecase, *memoryArtifactRepo, *authkeyTestClient) {
	t.Helper()

	credentialRepo := newMemoryCredentialRepo()
	artifactRepo := newMemoryArtifactRepo()
	credentialRepo.artifactRepo = artifactRepo
	client := &authkeyTestClient{}

	encryptedBlob, err := internalcrypto.EncryptString(testEncryptionKey, `{"account_id":"10001","cookie_token":"abc"}`)
	require.NoError(t, err)

	require.NoError(t, credentialRepo.Save(context.Background(), &biz.Credential{
		BindingRef:        "binding-101",
		AccountKey:        "hoyo_10001",
		Generation:        1,
		Platform:          "mihomo",
		AccountID:         "10001",
		Region:            "cn_gf01",
		CredentialBlob:    encryptedBlob,
		CredentialVersion: "v1",
		Status:            "active",
	}))

	artifacts := NewArtifactLifecycle(artifactRepo, ArtifactLifecycleConfig{Revoker: client, EncryptionKey: testEncryptionKey})
	return NewAuthkeyUsecase(credentialRepo, artifactRepo, artifacts, client, testEncryptionKey), artifactRepo, client
}

type memoryArtifactRepo struct {
	artifacts         map[string]*biz.Artifact
	revocationIntents map[string]*biz.ArtifactRevocationIntent
	deleteErr         error
}

func newMemoryArtifactRepo() *memoryArtifactRepo {
	return &memoryArtifactRepo{
		artifacts:         make(map[string]*biz.Artifact),
		revocationIntents: make(map[string]*biz.ArtifactRevocationIntent),
	}
}

func (r *memoryArtifactRepo) Put(_ context.Context, artifact *biz.Artifact) error {
	clone := *artifact
	r.artifacts[bindingArtifactKey(artifact.BindingRef, artifact.ArtifactType, artifact.ScopeKey)] = &clone
	return nil
}

func (r *memoryArtifactRepo) PutIfCredentialCurrent(_ context.Context, artifact *biz.Artifact, _ uint64) error {
	return r.Put(context.Background(), artifact)
}

func (r *memoryArtifactRepo) ListByBindingRef(_ context.Context, bindingRef string) ([]*biz.Artifact, error) {
	artifacts := make([]*biz.Artifact, 0)
	for _, artifact := range r.artifacts {
		if artifact.BindingRef == bindingRef {
			clone := *artifact
			artifacts = append(artifacts, &clone)
		}
	}
	return artifacts, nil
}

func (r *memoryArtifactRepo) HasRevocationPending(_ context.Context, bindingRef string) (bool, error) {
	for _, intent := range r.revocationIntents {
		if intent.BindingRef == bindingRef {
			return true, nil
		}
	}
	return false, nil
}

func (r *memoryArtifactRepo) PutRevocationIntentImmediately(_ context.Context, intent *biz.ArtifactRevocationIntent) (*biz.ArtifactRevocationIntent, error) {
	for _, existing := range r.revocationIntents {
		if existing.BindingRef == intent.BindingRef && existing.ArtifactType == intent.ArtifactType &&
			existing.ScopeKey == intent.ScopeKey && existing.ArtifactValue == intent.ArtifactValue {
			if intent.State == artifactIntentReady {
				existing.State = artifactIntentReady
				existing.ReadyAfter = intent.ReadyAfter
			}
			clone := *existing
			return &clone, nil
		}
	}
	clone := *intent
	r.revocationIntents[intent.IntentID] = &clone
	return &clone, nil
}

func (r *memoryArtifactRepo) MarkRevocationIntentReadyImmediately(_ context.Context, intentID string) error {
	if intent := r.revocationIntents[intentID]; intent != nil {
		intent.State = artifactIntentReady
		intent.ReadyAfter = time.Now().UTC()
		intent.LeaseToken = ""
		intent.LeaseExpiresAt = nil
	}
	return nil
}

func (r *memoryArtifactRepo) ClaimRevocationIntents(_ context.Context, now, leaseExpiresAt time.Time, leaseToken string) ([]*biz.ArtifactRevocationIntent, error) {
	intents := make([]*biz.ArtifactRevocationIntent, 0, len(r.revocationIntents))
	for _, intent := range r.revocationIntents {
		if intent.ReadyAfter.After(now) || (intent.LeaseExpiresAt != nil && intent.LeaseExpiresAt.After(now)) {
			continue
		}
		intent.LeaseToken = leaseToken
		intent.LeaseExpiresAt = &leaseExpiresAt
		clone := *intent
		intents = append(intents, &clone)
	}
	return intents, nil
}

func (r *memoryArtifactRepo) ResolveProvisionalRevocationIntent(_ context.Context, intentID, leaseToken string) (bool, error) {
	intent := r.revocationIntents[intentID]
	if intent == nil || intent.LeaseToken != leaseToken {
		return false, nil
	}
	artifact := r.artifacts[bindingArtifactKey(intent.BindingRef, intent.ArtifactType, intent.ScopeKey)]
	if artifact != nil && artifact.ArtifactValue == intent.ArtifactValue {
		delete(r.revocationIntents, intentID)
		return false, nil
	}
	intent.State = artifactIntentReady
	return true, nil
}

func (r *memoryArtifactRepo) ReleaseRevocationIntentClaim(_ context.Context, intentID, leaseToken string) error {
	if intent := r.revocationIntents[intentID]; intent != nil && intent.LeaseToken == leaseToken {
		intent.LeaseToken = ""
		intent.LeaseExpiresAt = nil
	}
	return nil
}

func (r *memoryArtifactRepo) FinalizeRevocationIntentImmediately(_ context.Context, intentID string) error {
	intent := r.revocationIntents[intentID]
	if intent == nil {
		return nil
	}
	if intent.State != artifactIntentProvisional {
		return biz.ErrArtifactRevocationPending
	}
	delete(r.revocationIntents, intentID)
	return nil
}

func (r *memoryArtifactRepo) DeleteRevocationIntentImmediately(_ context.Context, intentID string) error {
	delete(r.revocationIntents, intentID)
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
	if r.deleteErr != nil {
		return r.deleteErr
	}
	for key, artifact := range r.artifacts {
		if artifact.AccountKey == accountKey {
			delete(r.artifacts, key)
		}
	}
	return nil
}

func (r *memoryArtifactRepo) DeleteByBindingRef(_ context.Context, bindingRef string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	for key, artifact := range r.artifacts {
		if artifact.BindingRef == bindingRef {
			delete(r.artifacts, key)
		}
	}
	return nil
}

func (r *memoryArtifactRepo) DeleteByBindingRefImmediately(ctx context.Context, bindingRef string) error {
	return r.DeleteByBindingRef(ctx, bindingRef)
}

func (r *memoryArtifactRepo) DeleteArtifactImmediately(_ context.Context, bindingRef, artifactType, scopeKey, artifactValue string) error {
	key := bindingArtifactKey(bindingRef, artifactType, scopeKey)
	if artifact := r.artifacts[key]; artifact != nil && artifact.ArtifactValue == artifactValue {
		delete(r.artifacts, key)
	}
	return nil
}

func (r *memoryArtifactRepo) MarkRevocationPendingImmediately(context.Context, string) error {
	return nil
}

func (r *memoryArtifactRepo) DeleteExpired(_ context.Context, expiredBefore time.Time) (int64, error) {
	var deleted int64
	for key, artifact := range r.artifacts {
		if !artifact.ExpiresAt.After(expiredBefore) {
			delete(r.artifacts, key)
			deleted++
		}
	}
	return deleted, nil
}

type authkeyTestClient struct {
	issueAuthKeyCalls int
	issueErr          error
	expiresInSeconds  int64
	revokeErr         error
	revokeAttempts    int
	revoked           []string
}

func (c *authkeyTestClient) ValidateAndDiscover(_ context.Context, _ string, _ string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	return "", "", nil, nil
}

func (c *authkeyTestClient) IssueAuthKey(_ context.Context, cookieBundleJSON string, playerID string) (string, int64, error) {
	c.issueAuthKeyCalls++
	if c.issueErr != nil {
		return "", 0, c.issueErr
	}
	if cookieBundleJSON == "" {
		return "", 0, nil
	}
	return "issued-authkey-1", 300, nil
}

func (c *authkeyTestClient) IssueAuthKeyWithTTL(ctx context.Context, cookieBundleJSON string, playerID string, ttl time.Duration) (string, int64, error) {
	authKey, _, err := c.IssueAuthKey(ctx, cookieBundleJSON, playerID)
	expiresInSeconds := c.expiresInSeconds
	if expiresInSeconds == 0 {
		expiresInSeconds = int64(ttl / time.Second)
	}
	return authKey, expiresInSeconds, err
}

func (c *authkeyTestClient) RevokeAuthKey(_ context.Context, authKey string) error {
	c.revokeAttempts++
	if c.revokeErr != nil {
		return c.revokeErr
	}
	c.revoked = append(c.revoked, authKey)
	return nil
}

func artifactKey(accountKey, artifactType, scopeKey string) string {
	return accountKey + ":" + artifactType + ":" + scopeKey
}

func bindingArtifactKey(bindingRef string, artifactType, scopeKey string) string {
	return bindingRef + ":" + artifactType + ":" + scopeKey
}

var _ biz.ArtifactRepository = (*memoryArtifactRepo)(nil)
var _ platformmihomo.Client = (*authkeyTestClient)(nil)
