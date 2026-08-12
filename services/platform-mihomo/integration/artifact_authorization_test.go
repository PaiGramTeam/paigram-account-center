//go:build integration

package integration

import (
	"context"
	"testing"

	mihomov2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v2"
	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/usecase"
)

func TestAuthorizationFenceCommitsWhileAuthKeyRevocationRetries(t *testing.T) {
	stack := newIntegrationStack(t)
	upstream, simulator := newMihomoProtocolSimulator(t)
	control, runtime := newV2ClientsForTest(t, stack, upstream)

	bindResponse, err := control.BindCredential(
		ticketContext(t, "", "mihomo.credential.bind"),
		&platformv2.BindCredentialRequest{
			Operation:             operationRef("bind-op", platformv2.OperationKind_OPERATION_KIND_BIND_CREDENTIAL, 0, 1),
			CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"abc\"}","device_id":"device-integration","device_fp":"fingerprint-integration","device_name":"integration-device"}`,
		},
	)
	require.NoError(t, err)
	accountKey := bindResponse.GetResult().GetAccountKey()
	profiles, err := runtime.ListProfiles(
		ticketContext(t, accountKey, "mihomo.profile.read"),
		&mihomov2.ListProfilesRequest{Resource: bindingResource(accountKey)},
	)
	require.NoError(t, err)
	_, err = runtime.GetAuthKey(
		ticketContext(t, accountKey, "mihomo.authkey.issue"),
		&mihomov2.GetAuthKeyRequest{Resource: bindingResource(accountKey), ProfileRef: profiles.GetSnapshot().GetProfiles()[0].GetProfileRef()},
	)
	require.NoError(t, err)

	simulator.failNextRevocations(1)
	fenceOperation := authorizationFenceOperationRef("fence-retry-op", 1, "paigram-bot", 2, 1, 1, 1)
	response, err := control.ApplyAuthorizationFence(
		ticketContextForOperation(t, accountKey, "mihomo.authorization.fence.apply", contractticket.TypeControl, fenceOperation.GetOperationId(), 1),
		&platformv2.ApplyAuthorizationFenceRequest{
			Operation: fenceOperation, ConsumerPrincipal: "paigram-bot", MinimumGrantVersion: 2,
			MinimumOwnerEpoch: 1, MinimumConsumerEpoch: 1, MinimumEntryEpoch: 1,
		},
	)
	require.NoError(t, err)
	require.Equal(t, platformv2.OperationState_OPERATION_STATE_SUCCEEDED, response.GetResult().GetState())

	var minimumGrantVersion uint64
	require.NoError(t, stack.DB.Raw(
		"SELECT minimum_grant_version FROM authorization_fences WHERE binding_ref = ? AND consumer_principal = ?",
		integrationBindingRef,
		"paigram-bot",
	).Scan(&minimumGrantVersion).Error)
	require.Equal(t, uint64(2), minimumGrantVersion)
	var pendingCount int64
	require.NoError(t, stack.DB.Table("runtime_artifacts").Where("binding_ref = ? AND revocation_pending = TRUE", integrationBindingRef).Count(&pendingCount).Error)
	require.Equal(t, int64(1), pendingCount)

	artifacts := data.NewArtifactRepo(stack.DB, stack.Redis, stack.RedisPrefix)
	lifecycle := usecase.NewArtifactLifecycle(artifacts, usecase.ArtifactLifecycleConfig{Revoker: upstream, EncryptionKey: integrationEncryptionKey})
	require.NoError(t, lifecycle.RetryPending(context.Background()))
	requireArtifactCount(t, stack, 0)
	require.Equal(t, 1, simulator.revokedCount())
}

func TestCredentialGenerationCommitsWhileAuthKeyRevocationRetries(t *testing.T) {
	stack := newIntegrationStack(t)
	upstream, simulator := newMihomoProtocolSimulator(t)
	control, runtime := newV2ClientsForTest(t, stack, upstream)
	credentialPayload := `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"abc\"}","device_id":"device-integration","device_fp":"fingerprint-integration","device_name":"integration-device"}`
	bindResponse, err := control.BindCredential(
		ticketContext(t, "", "mihomo.credential.bind"),
		&platformv2.BindCredentialRequest{
			Operation:             operationRef("bind-op", platformv2.OperationKind_OPERATION_KIND_BIND_CREDENTIAL, 0, 1),
			CredentialPayloadJson: credentialPayload,
		},
	)
	require.NoError(t, err)
	accountKey := bindResponse.GetResult().GetAccountKey()
	profiles, err := runtime.ListProfiles(
		ticketContext(t, accountKey, "mihomo.profile.read"),
		&mihomov2.ListProfilesRequest{Resource: bindingResource(accountKey)},
	)
	require.NoError(t, err)
	_, err = runtime.GetAuthKey(
		ticketContext(t, accountKey, "mihomo.authkey.issue"),
		&mihomov2.GetAuthKeyRequest{Resource: bindingResource(accountKey), ProfileRef: profiles.GetSnapshot().GetProfiles()[0].GetProfileRef()},
	)
	require.NoError(t, err)

	simulator.failNextRevocations(1)
	replaceOperation := operationRef("replace-retry-op", platformv2.OperationKind_OPERATION_KIND_REPLACE_CREDENTIAL, 1, 2)
	response, err := control.ReplaceCredential(
		ticketContextForOperation(t, accountKey, "mihomo.credential.update", contractticket.TypeControl, replaceOperation.GetOperationId(), 1),
		&platformv2.ReplaceCredentialRequest{Operation: replaceOperation, AccountKey: accountKey, CredentialPayloadJson: credentialPayload},
	)
	require.NoError(t, err)
	require.Equal(t, platformv2.OperationState_OPERATION_STATE_SUCCEEDED, response.GetResult().GetState())
	var generation uint64
	require.NoError(t, stack.DB.Raw("SELECT generation FROM credential_records WHERE binding_ref = ?", integrationBindingRef).Scan(&generation).Error)
	require.Equal(t, uint64(2), generation)

	var intentCount int64
	require.NoError(t, stack.DB.Table("artifact_revocation_intents").Where("binding_ref = ?", integrationBindingRef).Count(&intentCount).Error)
	require.Equal(t, int64(1), intentCount)
	requireArtifactCount(t, stack, 0)

	artifacts := data.NewArtifactRepo(stack.DB, stack.Redis, stack.RedisPrefix)
	lifecycle := usecase.NewArtifactLifecycle(artifacts, usecase.ArtifactLifecycleConfig{Revoker: upstream, EncryptionKey: integrationEncryptionKey})
	require.NoError(t, lifecycle.RetryPending(context.Background()))
	require.NoError(t, stack.DB.Table("artifact_revocation_intents").Where("binding_ref = ?", integrationBindingRef).Count(&intentCount).Error)
	require.Zero(t, intentCount)
	require.Equal(t, 1, simulator.revokedCount())
}
