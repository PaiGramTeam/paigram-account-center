package service

import (
	"context"
	"testing"
	"time"

	mihomov1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mihomostub "platform-mihomo-service/internal/testkit/mihomostub"
	"platform-mihomo-service/internal/usecase"
)

type mihomoCredentialServiceTestHarness struct {
	service *MihomoCredentialService
	bindUC  *usecase.BindUsecase
}

func TestMihomoCredentialServiceGetCredentialSummary(t *testing.T) {
	harness := newMihomoCredentialServiceForTest(t)

	bindResp, err := harness.bindUC.BindCredential(context.Background(), usecase.BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)

	resp, err := harness.service.GetCredentialSummary(context.Background(), &mihomov1.GetCredentialSummaryRequest{
		ServiceTicket:     signedMihomoSummaryTicket(t, bindResp.PlatformAccountID, "mihomo.credential.read_meta"),
		PlatformAccountId: bindResp.PlatformAccountID,
	})
	require.NoError(t, err)
	require.Equal(t, bindResp.PlatformAccountID, resp.PlatformAccountId)
	require.NotEmpty(t, resp.Profiles)
	require.NotEmpty(t, resp.Devices)
}

func TestMihomoCredentialServiceGetCredentialSummaryRejectsMachineAccessToken(t *testing.T) {
	harness := newMihomoCredentialServiceForTest(t)

	_, err := harness.service.GetCredentialSummary(context.Background(), &mihomov1.GetCredentialSummaryRequest{
		ServiceTicket:     signedMihomoMachineAccessToken(t, "binding_101_10001", "mihomo.credential.read_meta"),
		PlatformAccountId: "binding_101_10001",
	})

	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestMihomoCredentialServiceRejectsTicketMissingPlatformAccountID(t *testing.T) {
	harness := newMihomoCredentialServiceForTest(t)

	_, err := harness.service.GetCredentialSummary(context.Background(), &mihomov1.GetCredentialSummaryRequest{
		ServiceTicket:     signedMihomoSummaryTicketWithoutAccount(t),
		PlatformAccountId: "binding_101_10001",
	})
	require.Error(t, err)
}

func TestMihomoCredentialServiceRejectsMissingSummaryScope(t *testing.T) {
	harness := newMihomoCredentialServiceForTest(t)

	bindResp, err := harness.bindUC.BindCredential(context.Background(), usecase.BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)

	_, err = harness.service.GetCredentialSummary(context.Background(), &mihomov1.GetCredentialSummaryRequest{
		ServiceTicket:     signedMihomoSummaryTicket(t, bindResp.PlatformAccountID),
		PlatformAccountId: bindResp.PlatformAccountID,
	})
	require.Error(t, err)
}

func TestMihomoCredentialServiceRejectsProfileScopedSummaryTicket(t *testing.T) {
	harness := newMihomoCredentialServiceForTest(t)

	bindResp, err := harness.bindUC.BindCredential(context.Background(), usecase.BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)

	_, err = harness.service.GetCredentialSummary(context.Background(), &mihomov1.GetCredentialSummaryRequest{
		ServiceTicket:     signedServiceTicketForProfile(t, bindResp.PlatformAccountID, 999, "mihomo.credential.read_meta"),
		PlatformAccountId: bindResp.PlatformAccountID,
	})
	require.Error(t, err)
}

func signedMihomoSummaryTicket(t *testing.T, platformAccountID string, scopes ...string) string {
	t.Helper()
	return signedServiceTicketForAccount(t, platformAccountID, scopes...)
}

func signedMihomoSummaryTicketWithoutAccount(t *testing.T) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":                  serviceTestIssuer,
		"aud":                  []string{serviceTestAudience},
		"actor_type":           "user",
		"actor_id":             "bot-paigram",
		"owner_user_id":        float64(1),
		"binding_id":           float64(101),
		"bot_id":               "bot-paigram",
		"platform":             "mihomo",
		"platform_service_key": serviceTestAudience,
		"scopes":               []string{"mihomo.credential.read_meta"},
		"exp":                  time.Now().Add(time.Minute).Unix(),
	}
	return signedServiceTestJWT(t, claims)
}

func signedMihomoMachineAccessToken(t *testing.T, platformAccountID string, scopes ...string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iss":                  serviceTestIssuer,
		"aud":                  []string{serviceTestAudience},
		"actor_type":           "machine",
		"actor_id":             "machine-paigram",
		"owner_user_id":        float64(1),
		"binding_id":           float64(101),
		"platform":             "mihomo",
		"platform_service_key": serviceTestAudience,
		"exp":                  time.Now().Add(time.Minute).Unix(),
	}
	if platformAccountID != "" {
		claims["platform_account_id"] = platformAccountID
	}
	if len(scopes) > 0 {
		claims["scopes"] = scopes
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = serviceTestKeyID
	token.Header["typ"] = "machine_access"
	signed, err := token.SignedString(serviceTestTicketPrivateKey)
	require.NoError(t, err)
	return signed
}

func newMihomoCredentialServiceForTest(t *testing.T) *mihomoCredentialServiceTestHarness {
	t.Helper()

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

	return &mihomoCredentialServiceTestHarness{
		service: NewMihomoCredentialService(
			serviceTestTicketVerifier(),
			managementUC,
		),
		bindUC: bindUC,
	}
}
