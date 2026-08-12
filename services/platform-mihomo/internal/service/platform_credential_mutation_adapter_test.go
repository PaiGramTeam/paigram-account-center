package service

import (
	"context"
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

func TestGenericPlatformServiceBindCredentialRejectsDelegationTicket(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())
	ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{
		ActorType:    "consumer",
		Consumer:     "paimon-bot",
		GrantVersion: 1,
		Scopes:       []string{"mihomo.credential.bind"},
	}))

	_, err := adapter.BindCredential(ctx, &platformv1.BindCredentialRequest{
		CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"initial\"}","device_id":"device-1","device_fp":"fingerprint-1"}`,
	})

	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGenericPlatformServiceExistingCredentialMutationsRejectDelegationTicket(t *testing.T) {
	tests := []struct {
		name   string
		action string
		call   func(*GenericPlatformService, context.Context, string) error
	}{
		{
			name:   "replace",
			action: "mihomo.credential.update",
			call: func(adapter *GenericPlatformService, ctx context.Context, platformAccountID string) error {
				_, err := adapter.ReplaceCredential(ctx, &platformv1.ReplaceCredentialRequest{
					PlatformAccountId:     platformAccountID,
					CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"replacement\"}","device_id":"device-2","device_fp":"fingerprint-2"}`,
				})
				return err
			},
		},
		{
			name:   "refresh",
			action: "mihomo.credential.refresh",
			call: func(adapter *GenericPlatformService, ctx context.Context, platformAccountID string) error {
				_, err := adapter.RefreshCredential(ctx, &platformv1.RefreshCredentialRequest{PlatformAccountId: platformAccountID})
				return err
			},
		},
		{
			name:   "delete",
			action: "mihomo.credential.delete",
			call: func(adapter *GenericPlatformService, ctx context.Context, platformAccountID string) error {
				_, err := adapter.DeleteCredential(ctx, &platformv1.DeleteCredentialRequest{PlatformAccountId: platformAccountID})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())
			bound, err := adapter.BindCredential(
				incomingServiceTicketContext(signedServiceTicketForAccount(t, "", "mihomo.credential.bind")),
				&platformv1.BindCredentialRequest{
					CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"initial\"}","device_id":"device-1","device_fp":"fingerprint-1"}`,
				},
			)
			require.NoError(t, err)
			platformAccountID := bound.GetSummary().GetPlatformAccountId()
			ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{
				ActorType:         "consumer",
				Consumer:          "paimon-bot",
				GrantVersion:      1,
				PlatformAccountID: platformAccountID,
				Scopes:            []string{test.action},
			}))

			err = test.call(adapter, ctx, platformAccountID)
			require.Equal(t, codes.PermissionDenied, status.Code(err))
		})
	}
}
