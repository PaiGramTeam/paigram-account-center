//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/data/model"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	platformserver "platform-mihomo-service/internal/server"
	"platform-mihomo-service/internal/usecase"
)

func TestArtifactRevocationFailureStaysHiddenAndRetries(t *testing.T) {
	stack := newIntegrationStack(t)
	credentials := data.NewCredentialRepo(stack.DB)
	artifacts := data.NewArtifactRepo(stack.DB, stack.Redis, stack.RedisPrefix)
	require.NoError(t, credentials.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-retry", AccountKey: "account-retry", Generation: 1, Platform: "mihomo",
		AccountID: "retry-account", Region: "cn_gf01", CredentialBlob: "ciphertext", CredentialVersion: "v1", Status: "active",
	}))
	ciphertext, err := internalcrypto.EncryptArtifact(integrationEncryptionKey, "retry-authkey", "binding-retry", "account-retry", "authkey", "1008611")
	require.NoError(t, err)
	require.NoError(t, artifacts.Put(context.Background(), &biz.Artifact{
		BindingRef: "binding-retry", AccountKey: "account-retry", ArtifactType: "authkey",
		ArtifactValue: ciphertext, ScopeKey: "1008611", ExpiresAt: time.Now().Add(time.Hour),
	}))
	revoker := &flakyAuthKeyRevoker{remainingFailures: 2}
	lifecycle := usecase.NewArtifactLifecycle(artifacts, usecase.ArtifactLifecycleConfig{Revoker: revoker, EncryptionKey: integrationEncryptionKey})

	require.Error(t, lifecycle.InvalidateBinding(context.Background(), "binding-retry"))
	pending, err := artifacts.HasRevocationPending(context.Background(), "binding-retry")
	require.NoError(t, err)
	require.True(t, pending)
	hidden, err := artifacts.GetByBindingRef(context.Background(), "binding-retry", "authkey", "1008611")
	require.NoError(t, err)
	require.Nil(t, hidden)
	keys, err := stack.Redis.Keys(context.Background(), stack.RedisPrefix+"artifact:binding:binding-retry:*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)
	staleCacheKey := stack.RedisPrefix + "artifact:binding:binding-retry:authkey:1008611"
	require.NoError(t, stack.Redis.Set(context.Background(), staleCacheKey, `{"ArtifactValue":"stale"}`, time.Hour).Err())
	hidden, err = artifacts.GetByBindingRef(context.Background(), "binding-retry", "authkey", "1008611")
	require.NoError(t, err)
	require.Nil(t, hidden)

	require.Error(t, lifecycle.RetryPending(context.Background()))
	var intentCount int64
	require.NoError(t, stack.DB.Table("artifact_revocation_intents").Where("binding_ref = ?", "binding-retry").Count(&intentCount).Error)
	require.Equal(t, int64(1), intentCount)
	require.NoError(t, lifecycle.RetryPending(context.Background()))
	pending, err = artifacts.HasRevocationPending(context.Background(), "binding-retry")
	require.NoError(t, err)
	require.False(t, pending)
	require.Equal(t, 3, revoker.attemptCount())
	require.Zero(t, stack.Redis.Exists(context.Background(), staleCacheKey).Val())
}

func TestDuplicateProvisionalIntentUpgradesToReadyAndRevokes(t *testing.T) {
	stack := newIntegrationStack(t)
	credentials := data.NewCredentialRepo(stack.DB)
	artifacts := data.NewArtifactRepo(stack.DB, stack.Redis, stack.RedisPrefix)
	require.NoError(t, credentials.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-upgrade", AccountKey: "account-upgrade", Generation: 1, Platform: "mihomo",
		AccountID: "upgrade-account", Region: "cn_gf01", CredentialBlob: "ciphertext", CredentialVersion: "v1", Status: "active",
	}))
	ciphertext, err := internalcrypto.EncryptArtifact(
		integrationEncryptionKey, "upgrade-authkey", "binding-upgrade", "account-upgrade", "authkey", "1008611",
	)
	require.NoError(t, err)
	artifact := &biz.Artifact{
		BindingRef: "binding-upgrade", AccountKey: "account-upgrade", ArtifactType: "authkey",
		ArtifactValue: ciphertext, ScopeKey: "1008611", ExpiresAt: time.Now().Add(time.Minute),
	}
	require.NoError(t, artifacts.Put(context.Background(), artifact))
	revoker := &flakyAuthKeyRevoker{}
	lifecycle := usecase.NewArtifactLifecycle(artifacts, usecase.ArtifactLifecycleConfig{Revoker: revoker, EncryptionKey: integrationEncryptionKey})
	provisional, err := lifecycle.StageIssuedArtifact(context.Background(), artifact)
	require.NoError(t, err)
	ready := *provisional
	ready.IntentID = "different-random-intent"
	ready.State = "ready"
	ready.ReadyAfter = time.Now().UTC()
	persisted, err := artifacts.PutRevocationIntentImmediately(context.Background(), &ready)
	require.NoError(t, err)
	require.Equal(t, provisional.IntentID, persisted.IntentID)
	require.Equal(t, "ready", persisted.State)
	require.ErrorIs(t, lifecycle.FinalizeIssuedArtifact(context.Background(), provisional.IntentID), biz.ErrArtifactRevocationPending)
	require.NoError(t, lifecycle.RetryPending(context.Background()))
	require.Equal(t, 1, revoker.attemptCount())
}

func TestArtifactCleanupServerRemovesExpiredDatabaseAndRedisEntries(t *testing.T) {
	stack := newIntegrationStack(t)
	credentials := data.NewCredentialRepo(stack.DB)
	artifacts := data.NewArtifactRepo(stack.DB, stack.Redis, stack.RedisPrefix)
	require.NoError(t, credentials.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-cleanup", AccountKey: "account-cleanup", Generation: 1, Platform: "mihomo",
		AccountID: "cleanup-account", Region: "cn_gf01", CredentialBlob: "ciphertext", CredentialVersion: "v1", Status: "active",
	}))
	require.NoError(t, artifacts.Put(context.Background(), &biz.Artifact{
		BindingRef: "binding-cleanup", AccountKey: "account-cleanup", ArtifactType: "authkey",
		ArtifactValue: "encrypted-artifact", ScopeKey: "1008611", ExpiresAt: time.Now().Add(-time.Minute),
	}))
	cacheKey := stack.RedisPrefix + "artifact:binding:binding-cleanup:authkey:1008611"
	require.NoError(t, stack.Redis.Set(context.Background(), cacheKey, `{"expired":true}`, time.Hour).Err())

	worker := platformserver.NewArtifactCleanupServer(usecase.NewArtifactLifecycle(artifacts), 5*time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- worker.Start(context.Background()) }()
	require.Eventually(t, func() bool {
		var count int64
		if stack.DB.Table("runtime_artifacts").Where("binding_ref = ?", "binding-cleanup").Count(&count).Error != nil {
			return false
		}
		return count == 0 && stack.Redis.Exists(context.Background(), cacheKey).Val() == 0
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, worker.Stop(context.Background()))
	require.NoError(t, <-done)
}

func TestAuthorizationFenceWaitsForInFlightAuthKeyAndRevokesIt(t *testing.T) {
	stack := newIntegrationStack(t)
	credentials := data.NewCredentialRepo(stack.DB)
	artifacts := data.NewArtifactRepo(stack.DB, stack.Redis, stack.RedisPrefix)
	client := newBlockingAuthKeyClient()
	lifecycle := usecase.NewArtifactLifecycle(artifacts, usecase.ArtifactLifecycleConfig{Revoker: client, EncryptionKey: integrationEncryptionKey})
	authkeys := usecase.NewAuthkeyUsecase(credentials, artifacts, lifecycle, client, integrationEncryptionKey)
	credentialBlob, err := internalcrypto.EncryptString(integrationEncryptionKey, `{"cookie_token":"valid"}`)
	require.NoError(t, err)
	require.NoError(t, credentials.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-artifact-race", AccountKey: "account-artifact-race", Generation: 1,
		Platform: "mihomo", AccountID: "10001", Region: "cn_gf01", CredentialBlob: credentialBlob,
		CredentialVersion: "v1", Status: "active",
	}))

	issueDone := make(chan error, 1)
	go func() {
		_, err := authkeys.GetAuthKey(context.Background(), "account-artifact-race", "1008611")
		issueDone <- err
	}()
	<-client.issueStarted

	fenceStarted := make(chan struct{})
	fenceDone := make(chan error, 1)
	go func() {
		close(fenceStarted)
		fenceDone <- credentials.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			if _, err := credentials.GetByBindingRefForUpdate(txCtx, "binding-artifact-race"); err != nil {
				return err
			}
			return lifecycle.InvalidateBinding(txCtx, "binding-artifact-race")
		})
	}()
	<-fenceStarted
	select {
	case err := <-fenceDone:
		t.Fatalf("fence completed before in-flight issuance released its binding lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(client.releaseIssue)
	require.NoError(t, <-issueDone)
	require.NoError(t, <-fenceDone)
	require.Equal(t, []string{"race-authkey"}, client.revokedAuthKeys())

	var count int64
	require.NoError(t, stack.DB.Table("runtime_artifacts").Where("binding_ref = ?", "binding-artifact-race").Count(&count).Error)
	require.Zero(t, count)
	keys, err := stack.Redis.Keys(context.Background(), stack.RedisPrefix+"artifact:binding:binding-artifact-race:*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)
}

func TestCleanupWaitsForProvisionalIssuanceAndKeepsCommittedAuthKey(t *testing.T) {
	stack := newIntegrationStack(t)
	credentials := data.NewCredentialRepo(stack.DB)
	artifacts := data.NewArtifactRepo(stack.DB, stack.Redis, stack.RedisPrefix)
	client := newBlockingAuthKeyClient()
	close(client.releaseIssue)
	lifecycle := usecase.NewArtifactLifecycle(artifacts, usecase.ArtifactLifecycleConfig{Revoker: client, EncryptionKey: integrationEncryptionKey})
	require.NoError(t, credentials.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-provisional", AccountKey: "account-provisional", Generation: 1,
		Platform: "mihomo", AccountID: "10001", Region: "cn_gf01", CredentialBlob: "ciphertext",
		CredentialVersion: "v1", Status: "active",
	}))
	ciphertext, err := internalcrypto.EncryptArtifact(
		integrationEncryptionKey, "provisional-authkey", "binding-provisional", "account-provisional", "authkey", "1008611",
	)
	require.NoError(t, err)
	artifact := &biz.Artifact{
		BindingRef: "binding-provisional", AccountKey: "account-provisional", ArtifactType: "authkey",
		ArtifactValue: ciphertext, ScopeKey: "1008611", ExpiresAt: time.Now().Add(time.Minute),
	}
	staged := make(chan error, 1)
	releaseCommit := make(chan struct{})
	issueDone := make(chan error, 1)
	go func() {
		issueDone <- credentials.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			if _, err := credentials.GetByBindingRefForUpdate(txCtx, artifact.BindingRef); err != nil {
				staged <- err
				return err
			}
			intent, err := lifecycle.StageIssuedArtifact(txCtx, artifact)
			if err != nil {
				staged <- err
				return err
			}
			if err := stack.DB.Model(&model.ArtifactRevocationIntent{}).Where("intent_id = ?", intent.IntentID).
				Update("ready_after", time.Now().Add(-time.Minute)).Error; err != nil {
				staged <- err
				return err
			}
			if err := artifacts.PutIfCredentialCurrent(txCtx, artifact, 1); err != nil {
				staged <- err
				return err
			}
			staged <- nil
			<-releaseCommit
			return nil
		})
	}()
	require.NoError(t, <-staged)
	retryDone := make(chan error, 1)
	go func() { retryDone <- lifecycle.RetryPending(context.Background()) }()
	select {
	case err := <-retryDone:
		t.Fatalf("cleanup completed before issuance transaction: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseCommit)
	require.NoError(t, <-issueDone)
	require.NoError(t, <-retryDone)
	require.Empty(t, client.revokedAuthKeys())
	stored, err := artifacts.GetByBindingRef(context.Background(), artifact.BindingRef, artifact.ArtifactType, artifact.ScopeKey)
	require.NoError(t, err)
	require.NotNil(t, stored)
	var intentCount int64
	require.NoError(t, stack.DB.Table("artifact_revocation_intents").Where("binding_ref = ?", artifact.BindingRef).Count(&intentCount).Error)
	require.Zero(t, intentCount)
}

func TestInvalidCredentialAdmissionHidesAuthKeyBeforeStatusCommit(t *testing.T) {
	stack := newIntegrationStack(t)
	credentials := data.NewCredentialRepo(stack.DB)
	profiles := data.NewProfileRepo(stack.DB)
	artifacts := data.NewArtifactRepo(stack.DB, stack.Redis, stack.RedisPrefix)
	client := newInvalidatingAuthKeyClient()
	lifecycle := usecase.NewArtifactLifecycle(artifacts, usecase.ArtifactLifecycleConfig{Revoker: client, EncryptionKey: integrationEncryptionKey})
	blob, err := internalcrypto.EncryptString(integrationEncryptionKey, `{"cookie_token":"invalid"}`)
	require.NoError(t, err)
	require.NoError(t, credentials.Save(context.Background(), &biz.Credential{
		BindingRef: "binding-invalid", AccountKey: "account-invalid", Generation: 1,
		Platform: "mihomo", AccountID: "10001", Region: "cn_gf01", CredentialBlob: blob,
		CredentialVersion: "v1", Status: "active",
	}))
	ciphertext, err := internalcrypto.EncryptArtifact(
		integrationEncryptionKey, "invalid-authkey", "binding-invalid", "account-invalid", "authkey", "1008611",
	)
	require.NoError(t, err)
	require.NoError(t, artifacts.Put(context.Background(), &biz.Artifact{
		BindingRef: "binding-invalid", AccountKey: "account-invalid", ArtifactType: "authkey",
		ArtifactValue: ciphertext, ScopeKey: "1008611", ExpiresAt: time.Now().Add(time.Minute),
	}))
	statusUC := usecase.NewStatusUsecase(credentials, profiles, client, integrationEncryptionKey, lifecycle)
	authkeyUC := usecase.NewAuthkeyUsecase(credentials, artifacts, lifecycle, client, integrationEncryptionKey)
	validateDone := make(chan error, 1)
	go func() {
		_, err := statusUC.ValidateCredential(context.Background(), "account-invalid")
		validateDone <- err
	}()
	<-client.revokeStarted
	confirmDone := make(chan error, 1)
	go func() { confirmDone <- authkeyUC.ConfirmDeliverable(context.Background(), "binding-invalid", nil) }()
	select {
	case err := <-confirmDone:
		t.Fatalf("delivery confirmation completed before invalid status committed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(client.releaseRevoke)
	require.NoError(t, <-validateDone)
	require.ErrorIs(t, <-confirmDone, usecase.ErrCredentialRequiresAttention)
}

type blockingAuthKeyClient struct {
	issueStarted chan struct{}
	releaseIssue chan struct{}
	startOnce    sync.Once
	mu           sync.Mutex
	revoked      []string
}

type flakyAuthKeyRevoker struct {
	mu                sync.Mutex
	remainingFailures int
	attempts          int
}

type invalidatingAuthKeyClient struct {
	revokeStarted chan struct{}
	releaseRevoke chan struct{}
	startOnce     sync.Once
}

func newInvalidatingAuthKeyClient() *invalidatingAuthKeyClient {
	return &invalidatingAuthKeyClient{revokeStarted: make(chan struct{}), releaseRevoke: make(chan struct{})}
}

func (c *invalidatingAuthKeyClient) ValidateAndDiscover(context.Context, string, string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	return "", "", nil, &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorExpiredCredential}
}

func (c *invalidatingAuthKeyClient) IssueAuthKey(context.Context, string, string) (string, int64, error) {
	return "", 0, errors.New("unexpected issue")
}

func (c *invalidatingAuthKeyClient) RevokeAuthKey(context.Context, string) error {
	c.startOnce.Do(func() { close(c.revokeStarted) })
	<-c.releaseRevoke
	return nil
}

func (r *flakyAuthKeyRevoker) RevokeAuthKey(context.Context, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts++
	if r.remainingFailures > 0 {
		r.remainingFailures--
		return errors.New("temporary revoke failure")
	}
	return nil
}

func (r *flakyAuthKeyRevoker) attemptCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts
}

func newBlockingAuthKeyClient() *blockingAuthKeyClient {
	return &blockingAuthKeyClient{issueStarted: make(chan struct{}), releaseIssue: make(chan struct{})}
}

func (c *blockingAuthKeyClient) ValidateAndDiscover(context.Context, string, string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	return "10001", "cn_gf01", simulatorProfiles(), nil
}

func (c *blockingAuthKeyClient) IssueAuthKey(ctx context.Context, cookieBundleJSON, playerID string) (string, int64, error) {
	return c.IssueAuthKeyWithTTL(ctx, cookieBundleJSON, playerID, 5*time.Minute)
}

func (c *blockingAuthKeyClient) IssueAuthKeyWithTTL(ctx context.Context, _ string, _ string, ttl time.Duration) (string, int64, error) {
	c.startOnce.Do(func() { close(c.issueStarted) })
	select {
	case <-ctx.Done():
		return "", 0, ctx.Err()
	case <-c.releaseIssue:
		return "race-authkey", int64(ttl / time.Second), nil
	}
}

func (c *blockingAuthKeyClient) RevokeAuthKey(_ context.Context, authKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.revoked = append(c.revoked, authKey)
	return nil
}

func (c *blockingAuthKeyClient) revokedAuthKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.revoked...)
}
