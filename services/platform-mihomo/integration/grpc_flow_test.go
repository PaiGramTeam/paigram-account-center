//go:build integration

package integration

import (
	"context"
	"crypto/ed25519"
	"net"
	"strings"
	"testing"
	"time"

	mihomov2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v2"
	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/operationid"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	"platform-mihomo-service/internal/data"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	"platform-mihomo-service/internal/service"
	mihomostub "platform-mihomo-service/internal/testkit/mihomostub"
	"platform-mihomo-service/internal/usecase"
)

const (
	integrationTicketIssuer   = "paigram-account-center"
	integrationTicketAudience = "platform-mihomo-service"
	integrationTicketKeyID    = "integration-service-ticket-key"
	integrationBindingRef     = "bind_integration_101"
	bufConnAddress            = "bufnet"
)

var integrationEncryptionKey = []byte("0123456789abcdef0123456789abcdef")
var integrationTicketPrivateKey = ed25519.NewKeyFromSeed([]byte("abcdef0123456789abcdef0123456789"))

func TestV2BindRuntimeAndDeleteFlow(t *testing.T) {
	stack := newIntegrationStack(t)
	control, runtime := newV2ClientsForTest(t, stack)
	descriptor, err := runtime.DescribePlatform(context.Background(), &mihomov2.DescribePlatformRequest{})
	require.NoError(t, err)
	require.Equal(t, "v2", descriptor.GetContractVersion())

	credentialPayload := `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"abc\"}","device_id":"device-integration","device_fp":"fingerprint-integration","device_name":"integration-device"}`
	bindOperation := operationRef("bind-op", platformv2.OperationKind_OPERATION_KIND_BIND_CREDENTIAL, 0, 1)
	bindRequest := &platformv2.BindCredentialRequest{
		Operation:             bindOperation,
		CredentialPayloadJson: credentialPayload,
	}
	bindResponse, err := control.BindCredential(ticketContext(t, "", "mihomo.credential.bind"), bindRequest)
	require.NoError(t, err)
	require.Equal(t, platformv2.OperationState_OPERATION_STATE_SUCCEEDED, bindResponse.GetResult().GetState())
	accountKey := bindResponse.GetResult().GetAccountKey()
	require.NotEmpty(t, accountKey)
	require.NotContains(t, accountKey, integrationBindingRef)

	profiles, err := runtime.ListProfiles(
		ticketContext(t, accountKey, "mihomo.profile.read"),
		&mihomov2.ListProfilesRequest{Resource: bindingResource(accountKey)},
	)
	require.NoError(t, err)
	require.True(t, profiles.GetSnapshot().GetComplete())
	require.Equal(t, uint64(1), profiles.GetSnapshot().GetRevision())
	require.Len(t, profiles.GetSnapshot().GetProfiles(), 2)
	profileRef := profiles.GetSnapshot().GetProfiles()[0].GetProfileRef()
	secondProfileRef := profiles.GetSnapshot().GetProfiles()[1].GetProfileRef()
	require.NotEmpty(t, profileRef)
	primary, err := runtime.GetPrimaryProfile(
		ticketContext(t, accountKey, "mihomo.profile.read"),
		&mihomov2.GetPrimaryProfileRequest{Resource: bindingResource(accountKey)},
	)
	require.NoError(t, err)
	require.Equal(t, profileRef, primary.GetProfile().GetProfileRef())
	primaryOperation := primaryProfileOperationRef("primary-op", 1, secondProfileRef, 1)
	selected, err := control.SetPrimaryProfile(
		ticketContextForOperation(t, accountKey, platformaction.MihomoProfilePrimarySet, contractticket.TypeControl, primaryOperation.GetOperationId(), primaryOperation.GetPreGeneration()),
		&platformv2.SetPrimaryProfileRequest{Operation: primaryOperation, AccountKey: accountKey, ProfileRef: secondProfileRef, ExpectedProfileRevision: 1},
	)
	require.NoError(t, err)
	require.Equal(t, uint64(2), selected.GetResult().GetProfileSnapshot().GetRevision())
	require.Equal(t, uint64(2), selected.GetResult().GetProfileSnapshot().GetObservedRevision())
	primary, err = runtime.GetPrimaryProfile(
		ticketContext(t, accountKey, "mihomo.profile.read"),
		&mihomov2.GetPrimaryProfileRequest{Resource: bindingResource(accountKey)},
	)
	require.NoError(t, err)
	require.Equal(t, secondProfileRef, primary.GetProfile().GetProfileRef())
	missingPrimaryOperation := primaryProfileOperationRef("missing-primary-op", 1, "profile-missing", 2)
	_, err = control.SetPrimaryProfile(
		ticketContextForOperation(t, accountKey, platformaction.MihomoProfilePrimarySet, contractticket.TypeControl, missingPrimaryOperation.GetOperationId(), missingPrimaryOperation.GetPreGeneration()),
		&platformv2.SetPrimaryProfileRequest{Operation: missingPrimaryOperation, AccountKey: accountKey, ProfileRef: "profile-missing", ExpectedProfileRevision: 2},
	)
	require.Equal(t, codes.NotFound, status.Code(err))
	profilesAfterRejectedPrimary, err := runtime.ListProfiles(
		ticketContext(t, accountKey, platformaction.MihomoProfileRead),
		&mihomov2.ListProfilesRequest{Resource: bindingResource(accountKey)},
	)
	require.NoError(t, err)
	require.Equal(t, uint64(2), profilesAfterRejectedPrimary.GetSnapshot().GetRevision())
	validated, err := runtime.ValidateCredential(
		ticketContext(t, accountKey, "mihomo.credential.validate"),
		&mihomov2.ValidateCredentialRequest{Resource: bindingResource(accountKey)},
	)
	require.NoError(t, err)
	require.Equal(t, platformv2.CredentialStatus_CREDENTIAL_STATUS_ACTIVE, validated.GetStatus())
	device, err := runtime.GetDevice(
		ticketContext(t, accountKey, "mihomo.device.read"),
		&mihomov2.GetDeviceRequest{Resource: bindingResource(accountKey), DeviceRef: usecase.FormatDeviceRef(accountKey, "device-integration")},
	)
	require.NoError(t, err)
	require.True(t, device.GetDevice().GetIsValid())

	_, err = runtime.ListProfiles(
		ticketContextForType(t, accountKey, "mihomo.profile.read", contractticket.TypeControl),
		&mihomov2.ListProfilesRequest{Resource: bindingResource(accountKey)},
	)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = control.GetBindingState(
		ticketContextForType(t, accountKey, "mihomo.binding.read", contractticket.TypeDelegation),
		&platformv2.GetBindingStateRequest{BindingRef: integrationBindingRef},
	)
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	authkey, err := runtime.GetAuthKey(
		ticketContext(t, accountKey, "mihomo.authkey.issue"),
		&mihomov2.GetAuthKeyRequest{Resource: bindingResource(accountKey), ProfileRef: profileRef},
	)
	require.NoError(t, err)
	require.NotEmpty(t, authkey.GetAuthkey())
	var storedAuthKey string
	require.NoError(t, stack.DB.Raw(
		"SELECT artifact_value FROM runtime_artifacts WHERE binding_ref = ? AND artifact_type = 'authkey'",
		integrationBindingRef,
	).Scan(&storedAuthKey).Error)
	require.NotEmpty(t, storedAuthKey)
	require.NotEqual(t, authkey.GetAuthkey(), storedAuthKey)
	keys, err := stack.Redis.Keys(context.Background(), stack.RedisPrefix+"artifact:binding:*").Result()
	require.NoError(t, err)
	require.NotEmpty(t, keys)
	cached, err := stack.Redis.Get(context.Background(), keys[0]).Result()
	require.NoError(t, err)
	require.False(t, strings.Contains(cached, authkey.GetAuthkey()))

	replayed, err := control.BindCredential(ticketContext(t, "", "mihomo.credential.bind"), bindRequest)
	require.NoError(t, err)
	require.Equal(t, accountKey, replayed.GetResult().GetAccountKey())

	changedPayload := credentialPayload + " "
	changedRequest := &platformv2.BindCredentialRequest{
		Operation:             operationRef("bind-op", platformv2.OperationKind_OPERATION_KIND_BIND_CREDENTIAL, 0, 1),
		CredentialPayloadJson: changedPayload,
	}
	_, err = control.BindCredential(ticketContext(t, "", "mihomo.credential.bind"), changedRequest)
	require.NoError(t, err)

	forgedRequest := proto.Clone(changedRequest).(*platformv2.BindCredentialRequest)
	forgedRequest.Operation.RequestFingerprint = "forged"
	_, err = control.BindCredential(ticketContext(t, "", "mihomo.credential.bind"), forgedRequest)
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	conflict := proto.Clone(bindRequest).(*platformv2.BindCredentialRequest)
	conflict.Operation = operationRef("bind-op", platformv2.OperationKind_OPERATION_KIND_BIND_CREDENTIAL, 1, 2)
	_, err = control.BindCredential(
		ticketContextForOperation(t, "", "mihomo.credential.bind", contractticket.TypeControl, conflict.Operation.GetOperationId(), conflict.Operation.GetPreGeneration()),
		conflict,
	)
	require.Equal(t, codes.AlreadyExists, status.Code(err))
	storedBind, err := control.GetOperation(
		ticketContextForOperation(t, accountKey, "mihomo.operation.read", contractticket.TypeControl, bindOperation.GetOperationId(), bindOperation.GetPreGeneration()),
		&platformv2.GetOperationRequest{OperationId: bindOperation.GetOperationId()},
	)
	require.NoError(t, err)
	require.Equal(t, platformv2.OperationState_OPERATION_STATE_SUCCEEDED, storedBind.GetResult().GetState())
	_, err = control.GetOperation(
		ticketContextForOperation(t, accountKey, "mihomo.operation.read", contractticket.TypeControl, "different-operation", bindOperation.GetPreGeneration()),
		&platformv2.GetOperationRequest{OperationId: bindOperation.GetOperationId()},
	)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	bindingState, err := control.GetBindingState(
		ticketContext(t, accountKey, "mihomo.binding.read"),
		&platformv2.GetBindingStateRequest{BindingRef: integrationBindingRef},
	)
	require.NoError(t, err)
	require.True(t, bindingState.GetState().GetExists())

	replacePayload := credentialPayload
	replaceOperation := operationRef("replace-op", platformv2.OperationKind_OPERATION_KIND_REPLACE_CREDENTIAL, 1, 2)
	replaced, err := control.ReplaceCredential(
		ticketContextForOperation(t, accountKey, "mihomo.credential.update", contractticket.TypeControl, replaceOperation.GetOperationId(), replaceOperation.GetPreGeneration()),
		&platformv2.ReplaceCredentialRequest{Operation: replaceOperation, AccountKey: accountKey, CredentialPayloadJson: replacePayload},
	)
	require.NoError(t, err)
	require.True(t, replaced.GetResult().GetProfileSnapshot().GetComplete())

	refreshOperation := operationRef("refresh-op", platformv2.OperationKind_OPERATION_KIND_REFRESH_CREDENTIAL, 2, 3)
	refreshed, err := control.RefreshCredential(
		ticketContextForOperation(t, accountKey, "mihomo.credential.refresh", contractticket.TypeControl, refreshOperation.GetOperationId(), refreshOperation.GetPreGeneration()),
		&platformv2.RefreshCredentialRequest{Operation: refreshOperation, AccountKey: accountKey},
	)
	require.NoError(t, err)
	require.Equal(t, platformv2.OperationState_OPERATION_STATE_SUCCEEDED, refreshed.GetResult().GetState())
	require.True(t, refreshed.GetResult().GetProfileSnapshot().GetComplete())
	refreshedState, err := control.GetBindingState(
		ticketContext(t, accountKey, "mihomo.binding.read"),
		&platformv2.GetBindingStateRequest{BindingRef: integrationBindingRef},
	)
	require.NoError(t, err)
	require.True(t, refreshedState.GetState().GetProfileSnapshot().GetComplete())
	require.Equal(t, refreshOperation.GetTargetGeneration(), refreshedState.GetState().GetProfileSnapshot().GetRevision())
	require.Equal(t, refreshOperation.GetTargetGeneration(), refreshedState.GetState().GetProfileSnapshot().GetObservedRevision())

	staleRefresh := operationRef("stale-refresh-op", platformv2.OperationKind_OPERATION_KIND_REFRESH_CREDENTIAL, 2, 3)
	_, err = control.RefreshCredential(
		ticketContextForOperation(t, accountKey, "mihomo.credential.refresh", contractticket.TypeControl, staleRefresh.GetOperationId(), staleRefresh.GetPreGeneration()),
		&platformv2.RefreshCredentialRequest{Operation: staleRefresh, AccountKey: accountKey},
	)
	require.Equal(t, codes.Aborted, status.Code(err))

	currentDelegation := ticketContextForDelegationGeneration(t, accountKey, "mihomo.status.read", 3)
	_, err = runtime.GetStatus(currentDelegation, &mihomov2.GetStatusRequest{Resource: bindingResource(accountKey)})
	require.NoError(t, err)
	fenceOperation := authorizationFenceOperationRef("fence-op", 3, "paigram-bot", 2, 1, 1, 1)
	fenced, err := control.ApplyAuthorizationFence(
		ticketContextForOperation(t, accountKey, "mihomo.authorization.fence.apply", contractticket.TypeControl, fenceOperation.GetOperationId(), fenceOperation.GetPreGeneration()),
		&platformv2.ApplyAuthorizationFenceRequest{Operation: fenceOperation, ConsumerPrincipal: "paigram-bot", MinimumGrantVersion: 2, MinimumOwnerEpoch: 1, MinimumConsumerEpoch: 1, MinimumEntryEpoch: 1},
	)
	require.NoError(t, err)
	require.Equal(t, platformv2.OperationState_OPERATION_STATE_SUCCEEDED, fenced.GetResult().GetState())
	_, err = runtime.GetStatus(currentDelegation, &mihomov2.GetStatusRequest{Resource: bindingResource(accountKey)})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	conflictingFenceOperation := authorizationFenceOperationRef("fence-op", 3, "paigram-bot", 3, 1, 1, 1)
	_, err = control.ApplyAuthorizationFence(
		ticketContextForOperation(t, accountKey, "mihomo.authorization.fence.apply", contractticket.TypeControl, conflictingFenceOperation.GetOperationId(), conflictingFenceOperation.GetPreGeneration()),
		&platformv2.ApplyAuthorizationFenceRequest{Operation: conflictingFenceOperation, ConsumerPrincipal: "paigram-bot", MinimumGrantVersion: 3, MinimumOwnerEpoch: 1, MinimumConsumerEpoch: 1, MinimumEntryEpoch: 1},
	)
	require.Equal(t, codes.AlreadyExists, status.Code(err))

	deleted, err := control.DeleteCredential(
		ticketContext(t, accountKey, "mihomo.credential.delete"),
		&platformv2.DeleteCredentialRequest{
			Operation:  operationRef("delete-op", platformv2.OperationKind_OPERATION_KIND_DELETE_CREDENTIAL, 3, 4),
			AccountKey: accountKey,
		},
	)
	require.NoError(t, err)
	require.Equal(t, platformv2.OperationState_OPERATION_STATE_SUCCEEDED, deleted.GetResult().GetState())
	absent, err := control.GetBindingState(
		ticketContext(t, accountKey, "mihomo.binding.read"),
		&platformv2.GetBindingStateRequest{BindingRef: integrationBindingRef},
	)
	require.NoError(t, err)
	require.False(t, absent.GetState().GetExists())

	alreadyAbsentOperation := operationRef("already-absent-delete-op", platformv2.OperationKind_OPERATION_KIND_DELETE_CREDENTIAL, 4, 5)
	alreadyAbsent, err := control.DeleteCredential(
		ticketContextForOperation(t, accountKey, "mihomo.credential.delete", contractticket.TypeControl, alreadyAbsentOperation.GetOperationId(), alreadyAbsentOperation.GetPreGeneration()),
		&platformv2.DeleteCredentialRequest{Operation: alreadyAbsentOperation, AccountKey: accountKey},
	)
	require.NoError(t, err)
	require.Equal(t, platformv2.OperationState_OPERATION_STATE_SUCCEEDED, alreadyAbsent.GetResult().GetState())

	_, err = runtime.GetStatus(
		ticketContext(t, accountKey, "mihomo.status.read"),
		&mihomov2.GetStatusRequest{Resource: bindingResource(accountKey)},
	)
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	lateOperation := operationRef("late-delete-op", platformv2.OperationKind_OPERATION_KIND_DELETE_CREDENTIAL, 4, 5)
	resolved, err := control.ResolveOperation(
		ticketContextForOperation(t, accountKey, "mihomo.operation.resolve", contractticket.TypeControl, lateOperation.GetOperationId(), lateOperation.GetPreGeneration()),
		&platformv2.ResolveOperationRequest{Operation: lateOperation},
	)
	require.NoError(t, err)
	require.Equal(t, platformv2.OperationState_OPERATION_STATE_NOT_RECEIVED, resolved.GetResult().GetState())
	late, err := control.DeleteCredential(
		ticketContextForOperation(t, accountKey, "mihomo.credential.delete", contractticket.TypeControl, lateOperation.GetOperationId(), lateOperation.GetPreGeneration()),
		&platformv2.DeleteCredentialRequest{Operation: lateOperation, AccountKey: accountKey},
	)
	require.NoError(t, err)
	require.Equal(t, platformv2.OperationState_OPERATION_STATE_NOT_RECEIVED, late.GetResult().GetState())
}

func newV2ClientsForTest(t *testing.T, stack *integrationStack) (platformv2.PlatformControlServiceClient, mihomov2.MihomoRuntimeServiceClient) {
	t.Helper()
	credentialRepo := data.NewCredentialRepo(stack.DB)
	deviceRepo := data.NewDeviceRepo(stack.DB)
	profileRepo := data.NewProfileRepo(stack.DB)
	artifactRepo := data.NewArtifactRepo(stack.DB, stack.Redis, stack.RedisPrefix)
	managementRepo := data.NewManagementRepo(stack.DB, stack.Redis, stack.RedisPrefix)
	grantRepo := data.NewGrantInvalidationRepo(stack.DB)
	client := mihomostub.Client{Profiles: []platformmihomo.DiscoveredProfile{
		{GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerID: "1008611", Nickname: "Traveler", Level: 60},
		{GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerID: "1008612", Nickname: "Aether", Level: 55},
	}}
	verifier := data.NewStaticKeyTicketVerifier(integrationTicketIssuer, integrationTicketKeyID, integrationTicketPrivateKey.Public().(ed25519.PublicKey)).
		WithGrantVersionLookup(grantRepo).
		WithAuthorizationStateLookup(data.NewTicketAuthorizationStateLookup(stack.DB))
	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, integrationEncryptionKey, artifactRepo)
	statusUC := usecase.NewStatusUsecase(credentialRepo, profileRepo, client, integrationEncryptionKey)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	managementUC := usecase.NewManagementUsecase(credentialRepo, deviceRepo, profileRepo, artifactRepo, managementRepo, bindUC, profileUC)
	controlService := service.NewPlatformControlService(
		verifier,
		usecase.NewOperationUsecase(data.NewOperationRepo(stack.DB)),
		bindUC,
		statusUC,
		profileUC,
		managementUC,
		credentialRepo,
		data.NewAuthorizationFenceRepo(stack.DB),
		grantRepo,
	)
	runtimeService := service.NewMihomoRuntimeService(
		verifier,
		statusUC,
		profileUC,
		usecase.NewAuthkeyUsecase(credentialRepo, artifactRepo, client, integrationEncryptionKey),
		managementUC,
		deviceRepo,
	)

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	platformv2.RegisterPlatformControlServiceServer(server, controlService)
	mihomov2.RegisterMihomoRuntimeServiceServer(server, runtimeService)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(
		ctx,
		bufConnAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return platformv2.NewPlatformControlServiceClient(conn), mihomov2.NewMihomoRuntimeServiceClient(conn)
}

func ticketContext(t *testing.T, accountKey string, action string) context.Context {
	ticketType := contractticket.TypeControl
	if platformaction.IsMihomoDelegationAction(action) {
		ticketType = contractticket.TypeDelegation
	}
	return ticketContextForType(t, accountKey, action, ticketType)
}

func ticketContextForDelegationGeneration(t *testing.T, accountKey, action string, generation uint64) context.Context {
	return ticketContextForOperation(t, accountKey, action, contractticket.TypeDelegation, "", generation)
}

func ticketContextForType(t *testing.T, accountKey string, action string, ticketType string) context.Context {
	operationID := ""
	credentialGeneration := uint64(0)
	switch action {
	case "mihomo.credential.bind":
		operationID = "bind-op"
	case "mihomo.credential.delete":
		operationID = "delete-op"
		credentialGeneration = 3
	}
	return ticketContextForOperation(t, accountKey, action, ticketType, operationID, credentialGeneration)
}

func ticketContextForOperation(t *testing.T, accountKey string, action string, ticketType string, operationID string, credentialGeneration uint64) context.Context {
	t.Helper()
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":            integrationTicketIssuer,
		"aud":            []string{integrationTicketAudience},
		"jti":            "ticket-" + action + "-" + now.Format(time.RFC3339Nano),
		"owner_user_ref": "usr-integration-1",
		"binding_ref":    integrationBindingRef,
		"platform":       "mihomo",
		"scopes":         []string{action},
		"iat":            now.Unix(),
		"nbf":            now.Add(-time.Second).Unix(),
		"exp":            now.Add(time.Minute).Unix(),
	}
	if ticketType == contractticket.TypeDelegation {
		claims["sub"] = "consumer:paigram-bot"
		claims["actor_type"] = "consumer"
		claims["actor_id"] = "paigram-bot"
		claims["consumer"] = "paigram-bot"
		claims["consumer_principal"] = "paigram-bot"
		claims["entry_identity_ref"] = "entry-integration-1"
		claims["grant_version"] = float64(1)
		claims["owner_epoch"] = float64(1)
		claims["consumer_epoch"] = float64(1)
		claims["entry_epoch"] = float64(1)
		if credentialGeneration == 0 {
			credentialGeneration = 1
		}
		claims["credential_generation"] = credentialGeneration
	} else {
		claims["sub"] = "user:usr-integration-1"
		claims["actor_type"] = "user"
		claims["actor_id"] = "integration-user-1"
		claims["credential_generation"] = credentialGeneration
		if operationID != "" {
			claims["operation_id"] = operationID
		}
	}
	if accountKey != "" {
		claims["account_key"] = accountKey
	}
	token := jwt.NewWithClaims(contractticket.SigningMethodEd25519, claims)
	token.Header["kid"] = integrationTicketKeyID
	token.Header["typ"] = ticketType
	raw, err := token.SignedString(integrationTicketPrivateKey)
	require.NoError(t, err)
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+raw))
}

func operationRef(id string, kind platformv2.OperationKind, pre uint64, target uint64) *platformv2.OperationRef {
	fingerprint := operationid.Fingerprint(kind.String(), integrationBindingRef, pre, target)
	return &platformv2.OperationRef{
		OperationId:        id,
		Kind:               kind,
		BindingRef:         integrationBindingRef,
		PreGeneration:      pre,
		TargetGeneration:   target,
		RequestFingerprint: fingerprint,
	}
}

func authorizationFenceOperationRef(id string, generation uint64, consumer string, minimumGrantVersion, minimumOwnerEpoch, minimumConsumerEpoch, minimumEntryEpoch uint64) *platformv2.OperationRef {
	return &platformv2.OperationRef{
		OperationId:      id,
		Kind:             platformv2.OperationKind_OPERATION_KIND_APPLY_AUTHORIZATION_FENCE,
		BindingRef:       integrationBindingRef,
		PreGeneration:    generation,
		TargetGeneration: generation,
		RequestFingerprint: operationid.AuthorizationFenceFingerprint(
			platformv2.OperationKind_OPERATION_KIND_APPLY_AUTHORIZATION_FENCE.String(), integrationBindingRef, consumer, generation,
			minimumGrantVersion, minimumOwnerEpoch, minimumConsumerEpoch, minimumEntryEpoch,
		),
	}
}

func primaryProfileOperationRef(id string, generation uint64, profileRef string, expectedRevision uint64) *platformv2.OperationRef {
	return &platformv2.OperationRef{
		OperationId:        id,
		Kind:               platformv2.OperationKind_OPERATION_KIND_SET_PRIMARY_PROFILE,
		BindingRef:         integrationBindingRef,
		PreGeneration:      generation,
		TargetGeneration:   generation,
		RequestFingerprint: operationid.PrimaryProfileFingerprint(platformv2.OperationKind_OPERATION_KIND_SET_PRIMARY_PROFILE.String(), integrationBindingRef, profileRef, generation, expectedRevision),
	}
}

func bindingResource(accountKey string) *platformv2.BindingResource {
	return &platformv2.BindingResource{BindingRef: integrationBindingRef, AccountKey: accountKey}
}
