package service

import (
	"testing"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGenericPlatformServiceReplaceCredentialUsesUpdateAction(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())
	bindContext := incomingServiceTicketContext(signedServiceTicketForAccount(t, "", "mihomo.credential.bind"))
	bound, err := adapter.BindCredential(bindContext, &platformv1.BindCredentialRequest{
		CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"initial\"}","device_id":"device-1","device_fp":"fingerprint-1","device_name":"iPhone","region_hint":"cn_gf01"}`,
	})
	require.NoError(t, err)
	platformAccountID := bound.GetSummary().GetPlatformAccountId()

	replaceContext := incomingServiceTicketContext(signedServiceTicketForAccount(t, platformAccountID, "mihomo.credential.update"))
	replaced, err := adapter.ReplaceCredential(replaceContext, &platformv1.ReplaceCredentialRequest{
		PlatformAccountId:     platformAccountID,
		CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"replacement\"}","device_id":"device-2","device_fp":"fingerprint-2","device_name":"iPad","region_hint":"cn_gf01"}`,
	})

	require.NoError(t, err)
	require.Equal(t, platformAccountID, replaced.GetSummary().GetPlatformAccountId())
}

func TestGenericPlatformServiceBindCredentialRejectsExistingBinding(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())
	bindContext := incomingServiceTicketContext(signedServiceTicketForAccount(t, "", "mihomo.credential.bind"))
	request := &platformv1.BindCredentialRequest{
		CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"initial\"}","device_id":"device-1","device_fp":"fingerprint-1","device_name":"iPhone","region_hint":"cn_gf01"}`,
	}
	_, err := adapter.BindCredential(bindContext, request)
	require.NoError(t, err)

	_, err = adapter.BindCredential(bindContext, request)
	require.Equal(t, codes.AlreadyExists, status.Code(err))
}
