package service

import (
	"context"
	"net"
	"testing"
	"time"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	mihomostub "platform-mihomo-service/internal/testkit/mihomostub"
	"platform-mihomo-service/internal/usecase"
)

func TestGenericPlatformServiceValidateCredentialRejectsStatusReadAction(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())
	ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{
		ActorType:         "consumer",
		Consumer:          "paimon-bot",
		GrantVersion:      1,
		PlatformAccountID: "binding_101_10001",
		Scopes:            []string{usecase.ActionStatusRead},
	}))

	_, err := adapter.ValidateCredential(ctx, &platformv1.ValidateCredentialRequest{PlatformAccountId: "binding_101_10001"})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGenericPlatformServiceConfirmPrimaryProfileRequiresWriteAction(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())
	ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{
		ActorType:         "user",
		PlatformAccountID: "binding_101_10001",
		Scopes:            []string{usecase.ActionProfileRead},
	}))

	_, err := adapter.ConfirmPrimaryProfile(ctx, &platformv1.ConfirmPrimaryProfileRequest{
		PlatformAccountId: "binding_101_10001",
		PlayerId:          "10001",
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGenericPlatformServiceInvalidateConsumerGrantRejectsConsumerTicket(t *testing.T) {
	store := newMemoryGrantInvalidationStore()
	adapter := newGenericPlatformServiceForAdapterTest(store)

	ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{ActorType: "consumer", Consumer: "paimon-bot", GrantVersion: 1, Scopes: []string{"mihomo.consumer_grant.invalidate"}}))
	_, err := adapter.InvalidateConsumerGrant(ctx, &platformv1.InvalidateConsumerGrantRequest{
		BindingId:           101,
		Consumer:            "paimon-bot",
		MinimumGrantVersion: 2,
	})

	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Empty(t, store.minimums)
}

func TestGenericPlatformServiceInvalidateConsumerGrantRejectsSystemTicket(t *testing.T) {
	store := newMemoryGrantInvalidationStore()
	adapter := newGenericPlatformServiceForAdapterTest(store)

	ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{ActorType: "system", Scopes: []string{"mihomo.consumer_grant.invalidate"}}))
	_, err := adapter.InvalidateConsumerGrant(ctx, &platformv1.InvalidateConsumerGrantRequest{
		BindingId:           101,
		Consumer:            "paimon-bot",
		MinimumGrantVersion: 2,
	})

	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Empty(t, store.minimums)
}

func TestGenericPlatformServiceInvalidateConsumerGrantStoresMinimumVersion(t *testing.T) {
	store := newMemoryGrantInvalidationStore()
	adapter := newGenericPlatformServiceForAdapterTest(store)

	ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{ActorType: "admin", Scopes: []string{"mihomo.consumer_grant.invalidate"}}))
	resp, err := adapter.InvalidateConsumerGrant(ctx, &platformv1.InvalidateConsumerGrantRequest{
		BindingId:           101,
		Consumer:            "paimon-bot",
		MinimumGrantVersion: 3,
	})

	require.NoError(t, err)
	require.True(t, resp.GetSuccess())
	require.Equal(t, uint64(3), store.minimums[grantInvalidationKey{bindingID: 101, consumer: "paimon-bot"}])
}

func TestGenericPlatformServiceInvalidateConsumerGrantRejectsMissingScope(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())

	ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{ActorType: "user", Scopes: []string{"mihomo.credential.read_meta"}}))
	_, err := adapter.InvalidateConsumerGrant(ctx, &platformv1.InvalidateConsumerGrantRequest{
		BindingId:           101,
		Consumer:            "paimon-bot",
		MinimumGrantVersion: 2,
	})

	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGenericPlatformServiceInvalidateConsumerGrantRejectsCrossPlatformTicket(t *testing.T) {
	store := newMemoryGrantInvalidationStore()
	adapter := newGenericPlatformServiceForAdapterTest(store)

	ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{ActorType: "admin", Platform: "starrail", Scopes: []string{"mihomo.consumer_grant.invalidate"}}))
	_, err := adapter.InvalidateConsumerGrant(ctx, &platformv1.InvalidateConsumerGrantRequest{
		BindingId:           101,
		Consumer:            "paimon-bot",
		MinimumGrantVersion: 2,
	})

	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Empty(t, store.minimums)
}

func TestGenericPlatformServiceInvalidateConsumerGrantRejectsProfileScopedTicket(t *testing.T) {
	store := newMemoryGrantInvalidationStore()
	adapter := newGenericPlatformServiceForAdapterTest(store)

	ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{ActorType: "admin", ProfileID: 1001, Scopes: []string{"mihomo.consumer_grant.invalidate"}}))
	_, err := adapter.InvalidateConsumerGrant(ctx, &platformv1.InvalidateConsumerGrantRequest{
		BindingId:           101,
		Consumer:            "paimon-bot",
		MinimumGrantVersion: 2,
	})

	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Empty(t, store.minimums)
}

func TestGenericPlatformServiceRejectsStaleConsumerTicketAfterGrantInvalidation(t *testing.T) {
	store := newMemoryGrantInvalidationStore()
	adapter := newGenericPlatformServiceForAdapterTest(store)

	adminCtx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{ActorType: "admin", Scopes: []string{"mihomo.consumer_grant.invalidate"}}))
	_, err := adapter.InvalidateConsumerGrant(adminCtx, &platformv1.InvalidateConsumerGrantRequest{
		BindingId:           101,
		Consumer:            "paimon-bot",
		MinimumGrantVersion: 5,
	})
	require.NoError(t, err)

	consumerCtx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{ActorType: "consumer", Consumer: "paimon-bot", GrantVersion: 4, PlatformAccountID: "binding_101_10001", Scopes: []string{"mihomo.credential.read_meta"}}))
	_, err = adapter.GetCredentialSummary(consumerCtx, &platformv1.GetCredentialSummaryRequest{
		PlatformAccountId: "binding_101_10001",
	})

	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

type grantInvalidationKey struct {
	bindingID uint64
	consumer  string
}

type memoryGrantInvalidationStore struct {
	minimums map[grantInvalidationKey]uint64
}

func newMemoryGrantInvalidationStore() *memoryGrantInvalidationStore {
	return &memoryGrantInvalidationStore{minimums: map[grantInvalidationKey]uint64{}}
}

func (m *memoryGrantInvalidationStore) Upsert(_ context.Context, bindingID uint64, consumer string, minimumVersion uint64) error {
	m.minimums[grantInvalidationKey{bindingID: bindingID, consumer: consumer}] = minimumVersion
	return nil
}

func (m *memoryGrantInvalidationStore) MinimumVersion(_ context.Context, bindingID uint64, consumer string) (uint64, error) {
	return m.minimums[grantInvalidationKey{bindingID: bindingID, consumer: consumer}], nil
}

func newGenericPlatformServiceForAdapterTest(store *memoryGrantInvalidationStore) *GenericPlatformService {
	credentialRepo := newMemoryCredentialRepo()
	deviceRepo := newMemoryDeviceRepo()
	profileRepo := newMemoryProfileRepo()
	artifactRepo := newMemoryArtifactRepo()
	client := mihomostub.Client{}

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, serviceTestSigningKey, artifactRepo)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	managementUC := usecase.NewManagementUsecase(
		credentialRepo,
		deviceRepo,
		profileRepo,
		artifactRepo,
		newMemoryManagementRepo(credentialRepo, deviceRepo, profileRepo, artifactRepo),
		bindUC,
		profileUC,
	)
	ticketVerifier := serviceTestTicketVerifier().WithGrantVersionLookup(store)
	return NewGenericPlatformService(ticketVerifier, bindUC, usecase.NewStatusUsecase(credentialRepo, client, serviceTestSigningKey), managementUC, store).
		WithConsumerUsecases(profileUC, nil)
}

type adapterTicketOptions struct {
	ActorType         string
	Consumer          string
	GrantVersion      uint64
	Platform          string
	PlatformAccountID string
	ProfileID         uint64
	Scopes            []string
}

func signedAdapterServiceTicket(t *testing.T, opts adapterTicketOptions) string {
	t.Helper()

	actorType := opts.ActorType
	if actorType == "" {
		actorType = "user"
	}
	platform := opts.Platform
	if platform == "" {
		platform = "mihomo"
	}
	claims := jwt.MapClaims{
		"iss":                  serviceTestIssuer,
		"aud":                  []string{serviceTestAudience},
		"actor_type":           actorType,
		"actor_id":             actorType + "-paigram",
		"owner_user_id":        float64(1),
		"binding_id":           float64(101),
		"platform":             platform,
		"platform_service_key": serviceTestAudience,
		"exp":                  time.Now().Add(time.Minute).Unix(),
	}
	if opts.Consumer != "" {
		claims["consumer"] = opts.Consumer
	}
	if opts.GrantVersion != 0 {
		claims["grant_version"] = float64(opts.GrantVersion)
	}
	if opts.PlatformAccountID != "" {
		claims["platform_account_id"] = opts.PlatformAccountID
	}
	if opts.ProfileID != 0 {
		claims["profile_id"] = float64(opts.ProfileID)
	}
	if len(opts.Scopes) > 0 {
		claims["scopes"] = opts.Scopes
	}

	return signedServiceTestJWT(t, claims)
}

func signedAdapterMachineAccessToken(t *testing.T, opts adapterTicketOptions) string {
	t.Helper()

	actorType := opts.ActorType
	if actorType == "" {
		actorType = "machine"
	}
	platform := opts.Platform
	if platform == "" {
		platform = "mihomo"
	}
	claims := jwt.MapClaims{
		"iss":                  serviceTestIssuer,
		"aud":                  []string{serviceTestAudience},
		"actor_type":           actorType,
		"actor_id":             actorType + "-paigram",
		"owner_user_id":        float64(1),
		"binding_id":           float64(101),
		"platform":             platform,
		"platform_service_key": serviceTestAudience,
		"exp":                  time.Now().Add(time.Minute).Unix(),
	}
	if opts.PlatformAccountID != "" {
		claims["platform_account_id"] = opts.PlatformAccountID
	}
	if len(opts.Scopes) > 0 {
		claims["scopes"] = opts.Scopes
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = serviceTestKeyID
	token.Header["typ"] = "machine_access"
	signed, err := token.SignedString(serviceTestTicketPrivateKey)
	require.NoError(t, err)
	return signed
}

func TestGenericPlatformServiceGetCredentialSummary(t *testing.T) {
	credentialRepo := newMemoryCredentialRepo()
	deviceRepo := newMemoryDeviceRepo()
	profileRepo := newMemoryProfileRepo()
	artifactRepo := newMemoryArtifactRepo()
	client := mihomostub.Client{}

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, serviceTestSigningKey, artifactRepo)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	managementUC := usecase.NewManagementUsecase(
		credentialRepo,
		deviceRepo,
		profileRepo,
		artifactRepo,
		newMemoryManagementRepo(credentialRepo, deviceRepo, profileRepo, artifactRepo),
		bindUC,
		profileUC,
	)
	adapter := NewGenericPlatformService(
		serviceTestTicketVerifier(),
		bindUC,
		usecase.NewStatusUsecase(credentialRepo, client, serviceTestSigningKey),
		managementUC,
		nil,
	).WithConsumerUsecases(profileUC, usecase.NewAuthkeyUsecase(credentialRepo, artifactRepo, client, serviceTestSigningKey))

	bindResp, err := bindUC.BindCredential(context.Background(), usecase.BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)

	summaryCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization",
		"Bearer "+signedMihomoSummaryTicket(t, bindResp.PlatformAccountID, "mihomo.credential.read_meta"),
	))
	resp, err := adapter.GetCredentialSummary(summaryCtx, &platformv1.GetCredentialSummaryRequest{
		PlatformAccountId: bindResp.PlatformAccountID,
	})
	require.NoError(t, err)
	require.Equal(t, bindResp.PlatformAccountID, resp.PlatformAccountId)
	require.NotEmpty(t, resp.Profiles)
	require.NotEmpty(t, resp.Devices)

	statusCtx := incomingServiceTicketContext(signedMihomoSummaryTicket(t, bindResp.PlatformAccountID, usecase.ActionStatusRead))
	statusResp, err := adapter.GetCredentialStatus(statusCtx, &platformv1.GetCredentialStatusRequest{
		PlatformAccountId: bindResp.PlatformAccountID,
	})
	require.NoError(t, err)
	require.Equal(t, platformv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE, statusResp.Status)

	validationCtx := incomingServiceTicketContext(signedMihomoSummaryTicket(t, bindResp.PlatformAccountID, usecase.ActionCredentialValidate))
	validationResp, err := adapter.ValidateCredential(validationCtx, &platformv1.ValidateCredentialRequest{
		PlatformAccountId: bindResp.PlatformAccountID,
	})
	require.NoError(t, err)
	require.Equal(t, platformv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE, validationResp.Status)

	profilesCtx := incomingServiceTicketContext(signedMihomoSummaryTicket(t, bindResp.PlatformAccountID, usecase.ActionProfileRead))
	profilesResp, err := adapter.ListProfiles(profilesCtx, &platformv1.ListProfilesRequest{
		PlatformAccountId: bindResp.PlatformAccountID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, profilesResp.Profiles)

	confirmCtx := incomingServiceTicketContext(signedMihomoSummaryTicket(t, bindResp.PlatformAccountID, usecase.ActionProfileWrite))
	confirmResp, err := adapter.ConfirmPrimaryProfile(confirmCtx, &platformv1.ConfirmPrimaryProfileRequest{
		PlatformAccountId: bindResp.PlatformAccountID,
		PlayerId:          profilesResp.Profiles[0].PlayerId,
	})
	require.NoError(t, err)
	require.Equal(t, profilesResp.Profiles[0].PlayerId, confirmResp.GetProfile().GetPlayerId())
	require.True(t, confirmResp.GetProfile().GetIsDefault())

	authkeyCtx := incomingServiceTicketContext(signedMihomoSummaryTicket(t, bindResp.PlatformAccountID, usecase.ActionAuthKeyIssue))
	authkeyResp, err := adapter.GetAuthKey(authkeyCtx, &platformv1.GetAuthKeyRequest{
		PlatformAccountId: bindResp.PlatformAccountID,
		PlayerId:          profilesResp.Profiles[0].PlayerId,
	})
	require.NoError(t, err)
	require.Equal(t, "stub-authkey", authkeyResp.Authkey)
}

func TestGenericPlatformServiceGetCredentialSummaryRejectsMachineAccessToken(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())

	ctx := incomingServiceTicketContext(signedAdapterMachineAccessToken(t, adapterTicketOptions{PlatformAccountID: "binding_101_10001", Scopes: []string{"mihomo.credential.read_meta"}}))
	_, err := adapter.GetCredentialSummary(ctx, &platformv1.GetCredentialSummaryRequest{
		PlatformAccountId: "binding_101_10001",
	})

	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGenericPlatformServiceBindCredentialRejectsMachineAccessToken(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())

	ctx := incomingServiceTicketContext(signedAdapterMachineAccessToken(t, adapterTicketOptions{Scopes: []string{"mihomo.credential.bind"}}))
	_, err := adapter.BindCredential(ctx, &platformv1.BindCredentialRequest{
		CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"abc\"}","device_id":"12345678-1234-1234-1234-123456789abc","device_fp":"abcdefghijklmn","device_name":"iPhone","region_hint":"cn_gf01"}`,
	})

	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGenericPlatformServiceRejectsMissingSummaryScope(t *testing.T) {
	credentialRepo := newMemoryCredentialRepo()
	deviceRepo := newMemoryDeviceRepo()
	profileRepo := newMemoryProfileRepo()
	artifactRepo := newMemoryArtifactRepo()
	client := mihomostub.Client{}

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, serviceTestSigningKey, artifactRepo)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	managementUC := usecase.NewManagementUsecase(
		credentialRepo,
		deviceRepo,
		profileRepo,
		artifactRepo,
		newMemoryManagementRepo(credentialRepo, deviceRepo, profileRepo, artifactRepo),
		bindUC,
		profileUC,
	)
	adapter := NewGenericPlatformService(
		serviceTestTicketVerifier(),
		bindUC,
		usecase.NewStatusUsecase(credentialRepo, client, serviceTestSigningKey),
		managementUC,
		nil,
	)

	bindResp, err := bindUC.BindCredential(context.Background(), usecase.BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)

	ctx := incomingServiceTicketContext(signedMihomoSummaryTicket(t, bindResp.PlatformAccountID))
	_, err = adapter.GetCredentialSummary(ctx, &platformv1.GetCredentialSummaryRequest{
		PlatformAccountId: bindResp.PlatformAccountID,
	})
	require.Error(t, err)
}

func TestGenericPlatformServiceRejectsProfileScopedSummaryTicket(t *testing.T) {
	credentialRepo := newMemoryCredentialRepo()
	deviceRepo := newMemoryDeviceRepo()
	profileRepo := newMemoryProfileRepo()
	artifactRepo := newMemoryArtifactRepo()
	client := mihomostub.Client{}

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, serviceTestSigningKey, artifactRepo)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	managementUC := usecase.NewManagementUsecase(
		credentialRepo,
		deviceRepo,
		profileRepo,
		artifactRepo,
		newMemoryManagementRepo(credentialRepo, deviceRepo, profileRepo, artifactRepo),
		bindUC,
		profileUC,
	)
	adapter := NewGenericPlatformService(
		serviceTestTicketVerifier(),
		bindUC,
		usecase.NewStatusUsecase(credentialRepo, client, serviceTestSigningKey),
		managementUC,
		nil,
	)

	bindResp, err := bindUC.BindCredential(context.Background(), usecase.BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)

	ctx := incomingServiceTicketContext(signedServiceTicketForProfile(t, bindResp.PlatformAccountID, 999, "mihomo.credential.read_meta"))
	_, err = adapter.GetCredentialSummary(ctx, &platformv1.GetCredentialSummaryRequest{
		PlatformAccountId: bindResp.PlatformAccountID,
	})
	require.Error(t, err)
}

func TestGenericPlatformServiceDescribePlatform(t *testing.T) {
	bindUC := usecase.NewBindUsecase(newMemoryCredentialRepo(), newMemoryDeviceRepo(), newMemoryProfileRepo(), mihomostub.Client{}, serviceTestSigningKey, newMemoryArtifactRepo())
	statusUC := usecase.NewStatusUsecase(newMemoryCredentialRepo(), mihomostub.Client{}, serviceTestSigningKey)
	adapter := NewGenericPlatformService(
		serviceTestTicketVerifier(),
		bindUC,
		statusUC,
		usecase.NewManagementUsecase(newMemoryCredentialRepo(), newMemoryDeviceRepo(), newMemoryProfileRepo(), newMemoryArtifactRepo(), newMemoryManagementRepo(newMemoryCredentialRepo(), newMemoryDeviceRepo(), newMemoryProfileRepo(), newMemoryArtifactRepo()), bindUC, usecase.NewProfileUsecase(newMemoryProfileRepo())),
		nil,
	)

	resp, err := adapter.DescribePlatform(context.Background(), &platformv1.DescribePlatformRequest{})
	require.NoError(t, err)
	require.Equal(t, "mihomo", resp.PlatformKey)
	require.Equal(t, "Mihomo", resp.DisplayName)
	require.Equal(t, serviceTicketAudience, resp.ServiceAudience)
	require.Equal(t, []string{"mihomo.status.read", "mihomo.credential.validate", "mihomo.profile.read", "mihomo.profile.write", "mihomo.authkey.issue", "mihomo.credential.read_meta", "mihomo.credential.bind", "mihomo.credential.update", "mihomo.credential.refresh", "mihomo.credential.delete", "mihomo.device.update", "mihomo.consumer_grant.invalidate"}, resp.SupportedActions)
	require.NotNil(t, resp.CredentialSchema)
	require.NotEmpty(t, resp.CredentialSchema.Fields)
}

func TestGenericPlatformServiceRegisteredOnGRPCServer(t *testing.T) {
	bindUC := usecase.NewBindUsecase(newMemoryCredentialRepo(), newMemoryDeviceRepo(), newMemoryProfileRepo(), mihomostub.Client{}, serviceTestSigningKey, newMemoryArtifactRepo())
	statusUC := usecase.NewStatusUsecase(newMemoryCredentialRepo(), mihomostub.Client{}, serviceTestSigningKey)
	adapter := NewGenericPlatformService(
		serviceTestTicketVerifier(),
		bindUC,
		statusUC,
		usecase.NewManagementUsecase(newMemoryCredentialRepo(), newMemoryDeviceRepo(), newMemoryProfileRepo(), newMemoryArtifactRepo(), newMemoryManagementRepo(newMemoryCredentialRepo(), newMemoryDeviceRepo(), newMemoryProfileRepo(), newMemoryArtifactRepo()), bindUC, usecase.NewProfileUsecase(newMemoryProfileRepo())),
		nil,
	)

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	platformv1.RegisterPlatformServiceServer(server, adapter)
	go server.Serve(listener)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	resp, err := platformv1.NewPlatformServiceClient(conn).DescribePlatform(context.Background(), &platformv1.DescribePlatformRequest{})
	require.NoError(t, err)
	require.Equal(t, "mihomo", resp.PlatformKey)
}

func TestGenericPlatformServiceBindCredentialUsesBindAction(t *testing.T) {
	credentialRepo := newMemoryCredentialRepo()
	deviceRepo := newMemoryDeviceRepo()
	profileRepo := newMemoryProfileRepo()
	artifactRepo := newMemoryArtifactRepo()
	client := mihomostub.Client{}

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, serviceTestSigningKey, artifactRepo)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	managementUC := usecase.NewManagementUsecase(
		credentialRepo,
		deviceRepo,
		profileRepo,
		artifactRepo,
		newMemoryManagementRepo(credentialRepo, deviceRepo, profileRepo, artifactRepo),
		bindUC,
		profileUC,
	)
	adapter := NewGenericPlatformService(
		serviceTestTicketVerifier(),
		bindUC,
		usecase.NewStatusUsecase(credentialRepo, client, serviceTestSigningKey),
		managementUC,
		nil,
	)

	ctx := incomingServiceTicketContext(signedMihomoSummaryTicket(t, "", "mihomo.credential.bind"))
	resp, err := adapter.BindCredential(ctx, &platformv1.BindCredentialRequest{
		CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"abc\"}","device_id":"12345678-1234-1234-1234-123456789abc","device_fp":"abcdefghijklmn","device_name":"iPhone","region_hint":"cn_gf01"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetSummary())
	require.Equal(t, "binding_101_10001", resp.GetSummary().GetPlatformAccountId())
}

func TestGenericPlatformServiceBindCredentialRejectsUpdateAction(t *testing.T) {
	credentialRepo := newMemoryCredentialRepo()
	deviceRepo := newMemoryDeviceRepo()
	profileRepo := newMemoryProfileRepo()
	artifactRepo := newMemoryArtifactRepo()
	client := mihomostub.Client{}

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, serviceTestSigningKey, artifactRepo)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	managementUC := usecase.NewManagementUsecase(
		credentialRepo,
		deviceRepo,
		profileRepo,
		artifactRepo,
		newMemoryManagementRepo(credentialRepo, deviceRepo, profileRepo, artifactRepo),
		bindUC,
		profileUC,
	)
	adapter := NewGenericPlatformService(
		serviceTestTicketVerifier(),
		bindUC,
		usecase.NewStatusUsecase(credentialRepo, client, serviceTestSigningKey),
		managementUC,
		nil,
	)

	ctx := incomingServiceTicketContext(signedMihomoSummaryTicket(t, "", "mihomo.credential.update"))
	_, err := adapter.BindCredential(ctx, &platformv1.BindCredentialRequest{
		CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"abc\"}","device_id":"12345678-1234-1234-1234-123456789abc","device_fp":"abcdefghijklmn","device_name":"iPhone","region_hint":"cn_gf01"}`,
	})
	require.Error(t, err)
}

func TestGenericPlatformServiceReplaceCredentialRejectsBindAction(t *testing.T) {
	credentialRepo := newMemoryCredentialRepo()
	deviceRepo := newMemoryDeviceRepo()
	profileRepo := newMemoryProfileRepo()
	artifactRepo := newMemoryArtifactRepo()
	client := mihomostub.Client{}

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, serviceTestSigningKey, artifactRepo)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	managementUC := usecase.NewManagementUsecase(
		credentialRepo,
		deviceRepo,
		profileRepo,
		artifactRepo,
		newMemoryManagementRepo(credentialRepo, deviceRepo, profileRepo, artifactRepo),
		bindUC,
		profileUC,
	)
	adapter := NewGenericPlatformService(
		serviceTestTicketVerifier(),
		bindUC,
		usecase.NewStatusUsecase(credentialRepo, client, serviceTestSigningKey),
		managementUC,
		nil,
	)

	bindResp, err := bindUC.BindCredential(context.Background(), usecase.BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)

	ctx := incomingServiceTicketContext(signedMihomoSummaryTicket(t, bindResp.PlatformAccountID, "mihomo.credential.bind"))
	_, err = adapter.ReplaceCredential(ctx, &platformv1.ReplaceCredentialRequest{
		PlatformAccountId:     bindResp.PlatformAccountID,
		CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"updated\"}","device_id":"device-2","device_fp":"fp-2","device_name":"iPad","region_hint":"cn_gf01"}`,
	})
	require.Error(t, err)
}

func TestGenericPlatformServiceDeleteCredentialUsesDeleteScope(t *testing.T) {
	credentialRepo := newMemoryCredentialRepo()
	deviceRepo := newMemoryDeviceRepo()
	profileRepo := newMemoryProfileRepo()
	artifactRepo := newMemoryArtifactRepo()
	client := mihomostub.Client{}

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, serviceTestSigningKey, artifactRepo)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	managementUC := usecase.NewManagementUsecase(
		credentialRepo,
		deviceRepo,
		profileRepo,
		artifactRepo,
		newMemoryManagementRepo(credentialRepo, deviceRepo, profileRepo, artifactRepo),
		bindUC,
		profileUC,
	)
	adapter := NewGenericPlatformService(
		serviceTestTicketVerifier(),
		bindUC,
		usecase.NewStatusUsecase(credentialRepo, client, serviceTestSigningKey),
		managementUC,
		nil,
	)

	bindResp, err := bindUC.BindCredential(context.Background(), usecase.BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)

	ctx := incomingServiceTicketContext(signedMihomoSummaryTicket(t, bindResp.PlatformAccountID, "mihomo.credential.delete"))
	resp, err := adapter.DeleteCredential(ctx, &platformv1.DeleteCredentialRequest{
		PlatformAccountId: bindResp.PlatformAccountID,
	})
	require.NoError(t, err)
	require.True(t, resp.GetSuccess())
}

func incomingServiceTicketContext(ticket string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+ticket))
}
