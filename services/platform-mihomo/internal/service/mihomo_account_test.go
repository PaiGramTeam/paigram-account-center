package service

import (
	"context"
	"crypto/ed25519"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "platform-mihomo-service/api/mihomo/v1"
	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	"platform-mihomo-service/internal/usecase"
)

func TestBindCredentialReturnsDiscoveredProfiles(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)

	bindResp, err := svc.BindCredential(context.Background(), &v1.BindCredentialRequest{
		ServiceTicket:    signedServiceTicket(t),
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device: &v1.DeviceInfo{
			DeviceId:   "12345678-1234-1234-1234-123456789abc",
			DeviceFp:   "abcdefghijklmn",
			DeviceName: "iPhone",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "binding_101_10001", bindResp.PlatformAccountId)
	require.Len(t, bindResp.Profiles, 1)

	upsertResp, err := svc.UpsertDevice(context.Background(), &v1.UpsertDeviceRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.device.update"),
		PlatformAccountId: bindResp.PlatformAccountId,
		Device:            &v1.DeviceInfo{DeviceId: "aaaaaaaa-1234-1234-1234-123456789abc", DeviceFp: "bbbbbbbbbbbbbb", DeviceName: "Android"},
	})
	require.NoError(t, err)
	require.True(t, upsertResp.Success)

	confirmResp, err := svc.ConfirmPrimaryProfile(context.Background(), &v1.ConfirmPrimaryProfileRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.profile.write"),
		PlatformAccountId: bindResp.PlatformAccountId,
		PlayerId:          "1008611",
	})
	require.NoError(t, err)
	require.NotNil(t, confirmResp.Profile)
	require.Equal(t, "1008611", confirmResp.Profile.PlayerId)

	statusResp, err := svc.GetCredentialStatus(context.Background(), &v1.GetCredentialStatusRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.status.read"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.NoError(t, err)
	require.Equal(t, v1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE, statusResp.Status)
	require.NotNil(t, statusResp.LastValidatedAt)

	validateResp, err := svc.ValidateCredential(context.Background(), &v1.ValidateCredentialRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.status.read"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.NoError(t, err)
	require.Equal(t, v1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE, validateResp.Status)
	require.Empty(t, validateResp.ErrorCode)

	refreshResp, err := svc.RefreshCredential(context.Background(), &v1.RefreshCredentialRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.credential.refresh"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.NoError(t, err)
	require.Equal(t, v1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE, refreshResp.Status)
	require.NotNil(t, refreshResp.RefreshedAt)

	profilesResp, err := svc.ListProfiles(context.Background(), &v1.ListProfilesRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.profile.read"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.NoError(t, err)
	require.Len(t, profilesResp.Profiles, 1)
	require.Equal(t, "1008611", profilesResp.Profiles[0].PlayerId)

	primaryResp, err := svc.GetPrimaryProfile(context.Background(), &v1.GetPrimaryProfileRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.profile.read"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.NoError(t, err)
	require.NotNil(t, primaryResp.Profile)
	require.Equal(t, "1008611", primaryResp.Profile.PlayerId)

	authkeyResp, err := svc.GetAuthKey(context.Background(), &v1.GetAuthKeyRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.authkey.issue"),
		PlatformAccountId: bindResp.PlatformAccountId,
		PlayerId:          primaryResp.Profile.PlayerId,
	})
	require.NoError(t, err)
	require.Equal(t, "stub-authkey", authkeyResp.Authkey)
	require.NotNil(t, authkeyResp.ExpiresAt)

	summaryResp, err := svc.GetCredentialSummary(context.Background(), &v1.GetCredentialSummaryRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.credential.read_meta"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.NoError(t, err)
	require.NotNil(t, summaryResp.Summary)
	require.Equal(t, bindResp.PlatformAccountId, summaryResp.Summary.PlatformAccountId)
	require.Len(t, summaryResp.Summary.Devices, 2)
	require.Len(t, summaryResp.Summary.Profiles, 1)
}

func TestUpdateCredentialReturnsUpdatedSummary(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp, err := svc.BindCredential(context.Background(), &v1.BindCredentialRequest{
		ServiceTicket:    signedServiceTicket(t),
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device:           &v1.DeviceInfo{DeviceId: "device-1", DeviceFp: "fp-1", DeviceName: "iPhone"},
	})
	require.NoError(t, err)

	updateResp, err := svc.UpdateCredential(context.Background(), &v1.UpdateCredentialRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.credential.update"),
		PlatformAccountId: bindResp.PlatformAccountId,
		CookieBundleJson:  `{"account_id":"10001","cookie_token":"updated"}`,
		Device:            &v1.DeviceInfo{DeviceId: "device-2", DeviceFp: "fp-2", DeviceName: "iPad"},
		RegionHint:        "cn_gf01",
	})
	require.NoError(t, err)
	require.NotNil(t, updateResp.Summary)
	require.Equal(t, bindResp.PlatformAccountId, updateResp.Summary.PlatformAccountId)
	require.Len(t, updateResp.Summary.Devices, 2)
	require.Equal(t, "device-2", updateResp.Summary.Devices[1].DeviceId)
}

func TestDeleteCredentialRemovesCredential(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp, err := svc.BindCredential(context.Background(), &v1.BindCredentialRequest{
		ServiceTicket:    signedServiceTicket(t),
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device:           &v1.DeviceInfo{DeviceId: "device-1", DeviceFp: "fp-1", DeviceName: "iPhone"},
	})
	require.NoError(t, err)

	deleteResp, err := svc.DeleteCredential(context.Background(), &v1.DeleteCredentialRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.credential.delete"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.NoError(t, err)
	require.True(t, deleteResp.Success)

	_, err = svc.GetCredentialSummary(context.Background(), &v1.GetCredentialSummaryRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.credential.read_meta"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetCredentialStatusRejectsOutOfScopePlatformAccountID(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	_, err := svc.GetCredentialStatus(context.Background(), &v1.GetCredentialStatusRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, "binding_101_10001", "mihomo.status.read"),
		PlatformAccountId: "binding_999_10001",
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestRefreshCredentialRejectsProfileScopedTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.RefreshCredential(context.Background(), &v1.RefreshCredentialRequest{
		ServiceTicket:     signedServiceTicketForProfile(t, bindResp.PlatformAccountId, 999, "mihomo.credential.refresh"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetCredentialStatusRejectsProfileScopedTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.GetCredentialStatus(context.Background(), &v1.GetCredentialStatusRequest{
		ServiceTicket:     signedServiceTicketForProfile(t, bindResp.PlatformAccountId, 999, "mihomo.status.read"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestValidateCredentialRejectsProfileScopedTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.ValidateCredential(context.Background(), &v1.ValidateCredentialRequest{
		ServiceTicket:     signedServiceTicketForProfile(t, bindResp.PlatformAccountId, 999, "mihomo.status.read"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestUpsertDeviceRejectsProfileScopedTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.UpsertDevice(context.Background(), &v1.UpsertDeviceRequest{
		ServiceTicket:     signedServiceTicketForProfile(t, bindResp.PlatformAccountId, 999, "mihomo.device.update"),
		PlatformAccountId: bindResp.PlatformAccountId,
		Device:            &v1.DeviceInfo{DeviceId: "aaaaaaaa-1234-1234-1234-123456789abc", DeviceFp: "bbbbbbbbbbbbbb"},
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestConfirmPrimaryProfileRejectsUnknownPlayerID(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp, err := svc.BindCredential(context.Background(), &v1.BindCredentialRequest{
		ServiceTicket:    signedServiceTicket(t),
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device:           &v1.DeviceInfo{DeviceId: "12345678-1234-1234-1234-123456789abc", DeviceFp: "abcdefghijklmn"},
	})
	require.NoError(t, err)

	_, err = svc.ConfirmPrimaryProfile(context.Background(), &v1.ConfirmPrimaryProfileRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.profile.write"),
		PlatformAccountId: bindResp.PlatformAccountId,
		PlayerId:          "not-found",
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestUpsertDeviceRejectsUnknownScopedAccount(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	_, err := svc.UpsertDevice(context.Background(), &v1.UpsertDeviceRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, "binding_101_missing", "mihomo.device.update"),
		PlatformAccountId: "binding_101_missing",
		Device:            &v1.DeviceInfo{DeviceId: "aaaaaaaa-1234-1234-1234-123456789abc", DeviceFp: "bbbbbbbbbbbbbb"},
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestDeleteCredentialRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp, err := svc.BindCredential(context.Background(), &v1.BindCredentialRequest{
		ServiceTicket:    signedServiceTicket(t),
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device:           &v1.DeviceInfo{DeviceId: "device-1", DeviceFp: "fp-1", DeviceName: "iPhone"},
	})
	require.NoError(t, err)

	_, err = svc.DeleteCredential(context.Background(), &v1.DeleteCredentialRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestDeleteCredentialRejectsProfileScopedTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.DeleteCredential(context.Background(), &v1.DeleteCredentialRequest{
		ServiceTicket:     signedServiceTicketForProfile(t, bindResp.PlatformAccountId, 999, "mihomo.credential.delete"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestBindCredentialRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)

	_, err := svc.BindCredential(context.Background(), &v1.BindCredentialRequest{
		ServiceTicket:    signedServiceTicketForAccount(t, ""),
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device:           &v1.DeviceInfo{DeviceId: "device-1", DeviceFp: "fp-1", DeviceName: "iPhone"},
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestBindCredentialRejectsProfileScopedTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)

	_, err := svc.BindCredential(context.Background(), &v1.BindCredentialRequest{
		ServiceTicket:    signedServiceTicketForProfile(t, "mihomo:101", 999, "mihomo.credential.bind"),
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device:           &v1.DeviceInfo{DeviceId: "device-1", DeviceFp: "fp-1", DeviceName: "iPhone"},
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetCredentialSummaryRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.GetCredentialSummary(context.Background(), &v1.GetCredentialSummaryRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetCredentialSummaryRejectsProfileScopedTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.GetCredentialSummary(context.Background(), &v1.GetCredentialSummaryRequest{
		ServiceTicket:     signedServiceTicketForProfile(t, bindResp.PlatformAccountId, 999, "mihomo.credential.read_meta"),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestUpdateCredentialRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.UpdateCredential(context.Background(), &v1.UpdateCredentialRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
		CookieBundleJson:  `{"account_id":"10001","cookie_token":"updated"}`,
		Device:            &v1.DeviceInfo{DeviceId: "device-2", DeviceFp: "fp-2", DeviceName: "iPad"},
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestUpdateCredentialRejectsProfileScopedTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.UpdateCredential(context.Background(), &v1.UpdateCredentialRequest{
		ServiceTicket:     signedServiceTicketForProfile(t, bindResp.PlatformAccountId, 999, "mihomo.credential.update"),
		PlatformAccountId: bindResp.PlatformAccountId,
		CookieBundleJson:  `{"account_id":"10001","cookie_token":"updated"}`,
		Device:            &v1.DeviceInfo{DeviceId: "device-2", DeviceFp: "fp-2", DeviceName: "iPad"},
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestListProfilesRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.ListProfiles(context.Background(), &v1.ListProfilesRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetPrimaryProfileRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.GetPrimaryProfile(context.Background(), &v1.GetPrimaryProfileRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetCredentialStatusRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.GetCredentialStatus(context.Background(), &v1.GetCredentialStatusRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestValidateCredentialRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.ValidateCredential(context.Background(), &v1.ValidateCredentialRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestRefreshCredentialRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.RefreshCredential(context.Background(), &v1.RefreshCredentialRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestUpsertDeviceRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.UpsertDevice(context.Background(), &v1.UpsertDeviceRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
		Device:            &v1.DeviceInfo{DeviceId: "aaaaaaaa-1234-1234-1234-123456789abc", DeviceFp: "bbbbbbbbbbbbbb"},
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestConfirmPrimaryProfileRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.ConfirmPrimaryProfile(context.Background(), &v1.ConfirmPrimaryProfileRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
		PlayerId:          bindResp.Profiles[0].PlayerId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestConfirmPrimaryProfileRejectsReadOnlyScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.ConfirmPrimaryProfile(context.Background(), &v1.ConfirmPrimaryProfileRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.profile.read"),
		PlatformAccountId: bindResp.PlatformAccountId,
		PlayerId:          bindResp.Profiles[0].PlayerId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestConfirmPrimaryProfileRejectsProfileScopedTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.ConfirmPrimaryProfile(context.Background(), &v1.ConfirmPrimaryProfileRequest{
		ServiceTicket:     signedServiceTicketForProfile(t, bindResp.PlatformAccountId, 1001, "mihomo.profile.write"),
		PlatformAccountId: bindResp.PlatformAccountId,
		PlayerId:          bindResp.Profiles[0].PlayerId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestConfirmPrimaryProfileRejectsExpiredTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	claims := jwt.MapClaims{
		"iss":                  serviceTestIssuer,
		"aud":                  []string{serviceTestAudience},
		"actor_type":           "bot",
		"actor_id":             "bot-paigram",
		"owner_user_id":        float64(1),
		"binding_id":           float64(101),
		"bot_id":               "bot-paigram",
		"platform":             "mihomo",
		"platform_service_key": serviceTestAudience,
		"platform_account_id":  bindResp.PlatformAccountId,
		"scopes":               []string{"mihomo.profile.write"},
		"exp":                  time.Now().Add(-time.Minute).Unix(),
	}
	expiredTicket := signedServiceTestJWT(t, claims)

	_, err := svc.ConfirmPrimaryProfile(context.Background(), &v1.ConfirmPrimaryProfileRequest{
		ServiceTicket:     expiredTicket,
		PlatformAccountId: bindResp.PlatformAccountId,
		PlayerId:          bindResp.Profiles[0].PlayerId,
	})
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGetAuthKeyRejectsMissingScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp := bindCredentialForServiceTest(t, svc)

	_, err := svc.GetAuthKey(context.Background(), &v1.GetAuthKeyRequest{
		ServiceTicket:     signedServiceTicketForAccount(t, bindResp.PlatformAccountId),
		PlatformAccountId: bindResp.PlatformAccountId,
		PlayerId:          bindResp.Profiles[0].PlayerId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetAuthKeyRejectsForeignProfileScope(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)
	bindResp, err := svc.BindCredential(context.Background(), &v1.BindCredentialRequest{
		ServiceTicket:    signedServiceTicket(t),
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device:           &v1.DeviceInfo{DeviceId: "device-1", DeviceFp: "fp-1", DeviceName: "iPhone"},
	})
	require.NoError(t, err)

	_, err = svc.GetAuthKey(context.Background(), &v1.GetAuthKeyRequest{
		ServiceTicket:     signedServiceTicketForProfile(t, bindResp.PlatformAccountId, 999, "mihomo.authkey.issue"),
		PlatformAccountId: bindResp.PlatformAccountId,
		PlayerId:          bindResp.Profiles[0].PlayerId,
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestBindCredentialRejectsInvalidServiceTicket(t *testing.T) {
	svc := newMihomoAccountServiceForTest(t)

	_, err := svc.BindCredential(context.Background(), &v1.BindCredentialRequest{
		ServiceTicket:    "invalid-ticket",
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device: &v1.DeviceInfo{
			DeviceId: "12345678-1234-1234-1234-123456789abc",
			DeviceFp: "abcdefghijklmn",
		},
	})
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

const (
	serviceTestAudience = "platform-mihomo-service"
	serviceTestIssuer   = "paigram-account-center"
	serviceTestKeyID    = "service-test-key"
)

var (
	serviceTestSigningKey       = []byte("0123456789abcdef0123456789abcdef")
	serviceTestTicketPrivateKey = ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))
)

func signedServiceTicket(t *testing.T) string {
	return signedServiceTicketForAccount(t, "", "mihomo.credential.bind")
}

func signedServiceTicketForAccount(t *testing.T, platformAccountID string, scopes ...string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iss":                  serviceTestIssuer,
		"aud":                  []string{serviceTestAudience},
		"actor_type":           "bot",
		"actor_id":             "bot-paigram",
		"owner_user_id":        float64(1),
		"binding_id":           float64(101),
		"bot_id":               "bot-paigram",
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
	return signedServiceTestJWT(t, claims)
}

func signedServiceTicketForProfile(t *testing.T, platformAccountID string, profileID uint64, scopes ...string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iss":                  serviceTestIssuer,
		"aud":                  []string{serviceTestAudience},
		"actor_type":           "bot",
		"actor_id":             "bot-paigram",
		"owner_user_id":        float64(1),
		"binding_id":           float64(101),
		"bot_id":               "bot-paigram",
		"platform":             "mihomo",
		"platform_service_key": serviceTestAudience,
		"platform_account_id":  platformAccountID,
		"profile_id":           float64(profileID),
		"exp":                  time.Now().Add(time.Minute).Unix(),
	}
	if len(scopes) > 0 {
		claims["scopes"] = scopes
	}

	return signedServiceTestJWT(t, claims)
}

func signedServiceTestJWT(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = serviceTestKeyID
	token.Header["typ"] = "service_ticket"
	signed, err := token.SignedString(serviceTestTicketPrivateKey)
	require.NoError(t, err)
	return signed
}

func serviceTestTicketVerifier() *data.TicketVerifier {
	return data.NewStaticKeyTicketVerifier(serviceTestIssuer, serviceTestKeyID, serviceTestTicketPrivateKey.Public().(ed25519.PublicKey))
}

func bindCredentialForServiceTest(t *testing.T, svc *MihomoAccountService) *v1.BindCredentialResponse {
	t.Helper()

	bindResp, err := svc.BindCredential(context.Background(), &v1.BindCredentialRequest{
		ServiceTicket:    signedServiceTicket(t),
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device:           &v1.DeviceInfo{DeviceId: "device-1", DeviceFp: "fp-1", DeviceName: "iPhone"},
	})
	require.NoError(t, err)
	return bindResp
}

func newMihomoAccountServiceForTest(t *testing.T) *MihomoAccountService {
	t.Helper()

	credentialRepo := newMemoryCredentialRepo()
	deviceRepo := newMemoryDeviceRepo()
	profileRepo := newMemoryProfileRepo()
	artifactRepo := newMemoryArtifactRepo()
	client := platformmihomo.StubClient{}

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, serviceTestSigningKey, artifactRepo)
	statusUC := usecase.NewStatusUsecase(credentialRepo, client, serviceTestSigningKey)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	authkeyUC := usecase.NewAuthkeyUsecase(credentialRepo, artifactRepo, client, serviceTestSigningKey)
	managementUC := usecase.NewManagementUsecase(
		credentialRepo,
		deviceRepo,
		profileRepo,
		artifactRepo,
		newMemoryManagementRepo(credentialRepo, deviceRepo, profileRepo, artifactRepo),
		bindUC,
		profileUC,
	)

	return NewMihomoAccountService(
		serviceTestTicketVerifier(),
		bindUC,
		statusUC,
		profileUC,
		authkeyUC,
		managementUC,
	)
}

type memoryManagementRepo struct {
	credentialRepo *memoryCredentialRepo
	deviceRepo     *memoryDeviceRepo
	profileRepo    *memoryProfileRepo
	artifactRepo   *memoryArtifactRepo
}

func newMemoryManagementRepo(
	credentialRepo *memoryCredentialRepo,
	deviceRepo *memoryDeviceRepo,
	profileRepo *memoryProfileRepo,
	artifactRepo *memoryArtifactRepo,
) *memoryManagementRepo {
	return &memoryManagementRepo{
		credentialRepo: credentialRepo,
		deviceRepo:     deviceRepo,
		profileRepo:    profileRepo,
		artifactRepo:   artifactRepo,
	}
}

func (r *memoryManagementRepo) DeleteCredentialGraph(_ context.Context, platformAccountID string) error {
	_ = r.credentialRepo.DeleteByPlatformAccountID(context.Background(), platformAccountID)
	_ = r.deviceRepo.DeleteByPlatformAccountID(context.Background(), platformAccountID)
	_ = r.profileRepo.DeleteByPlatformAccountID(context.Background(), platformAccountID)
	for key, artifact := range r.artifactRepo.artifacts {
		if artifact.PlatformAccountID == platformAccountID {
			delete(r.artifactRepo.artifacts, key)
		}
	}
	return nil
}

func (r *memoryManagementRepo) DeleteCredentialGraphByBindingID(_ context.Context, bindingID uint64) error {
	credential := r.credentialRepo.byBindingID[bindingID]
	if credential != nil {
		delete(r.credentialRepo.byPlatformAccountID, credential.PlatformAccountID)
		for key, artifact := range r.artifactRepo.artifacts {
			if artifact.PlatformAccountID == credential.PlatformAccountID {
				delete(r.artifactRepo.artifacts, key)
			}
		}
	}
	delete(r.credentialRepo.byBindingID, bindingID)
	_ = r.deviceRepo.DeleteByBindingID(context.Background(), bindingID)
	for _, profile := range r.profileRepo.byBindingID[bindingID] {
		delete(r.profileRepo.byPlatformAccountID, profile.PlatformAccountID)
	}
	delete(r.profileRepo.byBindingID, bindingID)
	return nil
}

type memoryCredentialRepo struct {
	byPlatformAccountID map[string]*biz.Credential
	byBindingID         map[uint64]*biz.Credential
}

func newMemoryCredentialRepo() *memoryCredentialRepo {
	return &memoryCredentialRepo{
		byPlatformAccountID: make(map[string]*biz.Credential),
		byBindingID:         make(map[uint64]*biz.Credential),
	}
}

func (r *memoryCredentialRepo) Save(_ context.Context, credential *biz.Credential) error {
	clone := *credential
	r.byPlatformAccountID[credential.PlatformAccountID] = &clone
	r.byBindingID[credential.BindingID] = &clone
	return nil
}

func (r *memoryCredentialRepo) GetByBindingID(_ context.Context, bindingID uint64) (*biz.Credential, error) {
	credential := r.byBindingID[bindingID]
	if credential == nil {
		return nil, nil
	}

	clone := *credential
	return &clone, nil
}

func (r *memoryCredentialRepo) GetByPlatformAccountID(_ context.Context, platformAccountID string) (*biz.Credential, error) {
	credential := r.byPlatformAccountID[platformAccountID]
	if credential == nil {
		return nil, nil
	}

	clone := *credential
	return &clone, nil
}

func (r *memoryCredentialRepo) DeleteByPlatformAccountID(_ context.Context, platformAccountID string) error {
	if credential := r.byPlatformAccountID[platformAccountID]; credential != nil {
		delete(r.byBindingID, credential.BindingID)
	}
	delete(r.byPlatformAccountID, platformAccountID)
	return nil
}

type memoryDeviceRepo struct {
	byPlatformAccountID map[string][]*biz.Device
	byBindingID         map[uint64][]*biz.Device
}

func newMemoryDeviceRepo() *memoryDeviceRepo {
	return &memoryDeviceRepo{
		byPlatformAccountID: make(map[string][]*biz.Device),
		byBindingID:         make(map[uint64][]*biz.Device),
	}
}

func (r *memoryDeviceRepo) Save(_ context.Context, device *biz.Device) error {
	clone := *device
	current := r.byPlatformAccountID[device.PlatformAccountID]
	byBinding := r.byBindingID[device.BindingID]
	for index, existing := range current {
		if existing.DeviceID == device.DeviceID {
			current[index] = &clone
			r.byPlatformAccountID[device.PlatformAccountID] = current
			for bindingIndex, bindingDevice := range byBinding {
				if bindingDevice.DeviceID == device.DeviceID {
					byBinding[bindingIndex] = &clone
					r.byBindingID[device.BindingID] = byBinding
					return nil
				}
			}
			return nil
		}
	}

	r.byPlatformAccountID[device.PlatformAccountID] = append(current, &clone)
	r.byBindingID[device.BindingID] = append(byBinding, &clone)
	return nil
}

func (r *memoryDeviceRepo) ListByBindingID(_ context.Context, bindingID uint64) ([]*biz.Device, error) {
	devices := r.byBindingID[bindingID]
	result := make([]*biz.Device, 0, len(devices))
	for _, device := range devices {
		clone := *device
		result = append(result, &clone)
	}

	return result, nil
}

func (r *memoryDeviceRepo) ListByPlatformAccountID(_ context.Context, platformAccountID string) ([]*biz.Device, error) {
	devices := r.byPlatformAccountID[platformAccountID]
	result := make([]*biz.Device, 0, len(devices))
	for _, device := range devices {
		clone := *device
		result = append(result, &clone)
	}

	return result, nil
}

func (r *memoryDeviceRepo) DeleteByPlatformAccountID(_ context.Context, platformAccountID string) error {
	if devices := r.byPlatformAccountID[platformAccountID]; len(devices) > 0 {
		bindingID := devices[0].BindingID
		current := r.byBindingID[bindingID]
		filtered := make([]*biz.Device, 0, len(current))
		for _, device := range current {
			if device.PlatformAccountID != platformAccountID {
				filtered = append(filtered, device)
			}
		}
		if len(filtered) == 0 {
			delete(r.byBindingID, bindingID)
		} else {
			r.byBindingID[bindingID] = filtered
		}
	}
	delete(r.byPlatformAccountID, platformAccountID)
	return nil
}

func (r *memoryDeviceRepo) DeleteByBindingID(_ context.Context, bindingID uint64) error {
	for _, device := range r.byBindingID[bindingID] {
		delete(r.byPlatformAccountID, device.PlatformAccountID)
	}
	delete(r.byBindingID, bindingID)
	return nil
}

type memoryProfileRepo struct {
	byPlatformAccountID map[string][]*biz.Profile
	byBindingID         map[uint64][]*biz.Profile
}

func newMemoryProfileRepo() *memoryProfileRepo {
	return &memoryProfileRepo{
		byPlatformAccountID: make(map[string][]*biz.Profile),
		byBindingID:         make(map[uint64][]*biz.Profile),
	}
}

func (r *memoryProfileRepo) Save(_ context.Context, profile *biz.Profile) error {
	clone := *profile
	current := r.byPlatformAccountID[profile.PlatformAccountID]
	byBinding := r.byBindingID[profile.BindingID]
	for index, existing := range current {
		if existing.PlayerID == profile.PlayerID && existing.Region == profile.Region {
			current[index] = &clone
			r.byPlatformAccountID[profile.PlatformAccountID] = current
			for bindingIndex, bindingProfile := range byBinding {
				if bindingProfile.PlayerID == profile.PlayerID && bindingProfile.Region == profile.Region {
					byBinding[bindingIndex] = &clone
					r.byBindingID[profile.BindingID] = byBinding
					return nil
				}
			}
			return nil
		}
	}

	r.byPlatformAccountID[profile.PlatformAccountID] = append(current, &clone)
	r.byBindingID[profile.BindingID] = append(byBinding, &clone)
	return nil
}

func (r *memoryProfileRepo) ListByBindingID(_ context.Context, bindingID uint64) ([]*biz.Profile, error) {
	profiles := r.byBindingID[bindingID]
	result := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		clone := *profile
		result = append(result, &clone)
	}

	return result, nil
}

func (r *memoryProfileRepo) ListByPlatformAccountID(_ context.Context, platformAccountID string) ([]*biz.Profile, error) {
	profiles := r.byPlatformAccountID[platformAccountID]
	result := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		clone := *profile
		result = append(result, &clone)
	}

	return result, nil
}

func (r *memoryProfileRepo) SetDefaultByBindingAndPlayerID(_ context.Context, bindingID uint64, platformAccountID string, playerID string) error {
	for _, profile := range r.byPlatformAccountID[platformAccountID] {
		if profile.BindingID == bindingID {
			profile.IsDefault = profile.PlayerID == playerID
		}
	}
	for _, profile := range r.byBindingID[bindingID] {
		if profile.PlatformAccountID == platformAccountID {
			profile.IsDefault = profile.PlayerID == playerID
		}
	}
	return nil
}

func (r *memoryProfileRepo) DeleteByPlatformAccountID(_ context.Context, platformAccountID string) error {
	if profiles := r.byPlatformAccountID[platformAccountID]; len(profiles) > 0 {
		bindingID := profiles[0].BindingID
		current := r.byBindingID[bindingID]
		filtered := make([]*biz.Profile, 0, len(current))
		for _, profile := range current {
			if profile.PlatformAccountID != platformAccountID {
				filtered = append(filtered, profile)
			}
		}
		if len(filtered) == 0 {
			delete(r.byBindingID, bindingID)
		} else {
			r.byBindingID[bindingID] = filtered
		}
	}
	delete(r.byPlatformAccountID, platformAccountID)
	return nil
}

func (r *memoryProfileRepo) DeleteMissingByPlatformAccountID(_ context.Context, platformAccountID string, keep []biz.ProfileIdentity) error {
	profiles := r.byPlatformAccountID[platformAccountID]
	if len(profiles) == 0 {
		return nil
	}
	bindingID := profiles[0].BindingID
	keepSet := make(map[string]struct{}, len(keep))
	for _, identity := range keep {
		keepSet[identity.PlayerID+":"+identity.Region] = struct{}{}
	}
	filtered := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		if _, ok := keepSet[profile.PlayerID+":"+profile.Region]; ok {
			filtered = append(filtered, profile)
		}
	}
	r.byPlatformAccountID[platformAccountID] = filtered
	current := r.byBindingID[bindingID]
	filteredByBinding := make([]*biz.Profile, 0, len(current))
	for _, profile := range current {
		if profile.PlatformAccountID != platformAccountID {
			filteredByBinding = append(filteredByBinding, profile)
			continue
		}
		if _, ok := keepSet[profile.PlayerID+":"+profile.Region]; ok {
			filteredByBinding = append(filteredByBinding, profile)
		}
	}
	if len(filteredByBinding) == 0 {
		delete(r.byBindingID, bindingID)
	} else {
		r.byBindingID[bindingID] = filteredByBinding
	}
	if len(filtered) == 0 {
		delete(r.byPlatformAccountID, platformAccountID)
	}
	return nil
}

func (r *memoryProfileRepo) DeleteMissingByBindingID(_ context.Context, bindingID uint64, keep []biz.ProfileIdentity) error {
	profiles := r.byBindingID[bindingID]
	keepSet := make(map[string]struct{}, len(keep))
	for _, identity := range keep {
		keepSet[identity.PlayerID+":"+identity.Region] = struct{}{}
	}
	filtered := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		if _, ok := keepSet[profile.PlayerID+":"+profile.Region]; ok {
			filtered = append(filtered, profile)
		}
	}
	r.byBindingID[bindingID] = filtered
	for platformAccountID, platformProfiles := range r.byPlatformAccountID {
		filteredByAccount := platformProfiles[:0]
		for _, profile := range platformProfiles {
			if profile.BindingID != bindingID {
				filteredByAccount = append(filteredByAccount, profile)
				continue
			}
			if _, ok := keepSet[profile.PlayerID+":"+profile.Region]; ok {
				filteredByAccount = append(filteredByAccount, profile)
			}
		}
		if len(filteredByAccount) == 0 {
			delete(r.byPlatformAccountID, platformAccountID)
		} else {
			r.byPlatformAccountID[platformAccountID] = filteredByAccount
		}
	}
	return nil
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

func artifactKey(platformAccountID, artifactType, scopeKey string) string {
	return platformAccountID + ":" + artifactType + ":" + scopeKey
}

func bindingArtifactKey(bindingID uint64, artifactType, scopeKey string) string {
	return strconv.FormatUint(bindingID, 10) + ":" + artifactType + ":" + scopeKey
}

var _ biz.CredentialRepository = (*memoryCredentialRepo)(nil)
var _ biz.DeviceRepository = (*memoryDeviceRepo)(nil)
var _ biz.ProfileRepository = (*memoryProfileRepo)(nil)
var _ biz.ArtifactRepository = (*memoryArtifactRepo)(nil)
var _ biz.CredentialManagementRepository = (*memoryManagementRepo)(nil)
var _ platformmihomo.Client = platformmihomo.StubClient{}
