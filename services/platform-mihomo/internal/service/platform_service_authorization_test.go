package service

import (
	"testing"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	"platform-mihomo-service/internal/usecase"
)

func TestGenericPlatformServiceConfirmPrimaryProfileRejectsDelegationTicket(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())
	ctx := incomingServiceTicketContext(signedAdapterServiceTicket(t, adapterTicketOptions{
		ActorType:         "consumer",
		Consumer:          "paimon-bot",
		GrantVersion:      1,
		PlatformAccountID: "binding_101_10001",
		Scopes:            []string{usecase.ActionProfileWrite},
	}))

	_, err := adapter.ConfirmPrimaryProfile(ctx, &platformv1.ConfirmPrimaryProfileRequest{
		PlatformAccountId: "binding_101_10001",
		PlayerId:          "10001",
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestMapUsecaseErrorPreservesTypedUpstreamSemantics(t *testing.T) {
	tests := []struct {
		kind platformmihomo.ErrorKind
		want codes.Code
	}{
		{kind: platformmihomo.ErrorRateLimited, want: codes.ResourceExhausted},
		{kind: platformmihomo.ErrorUnavailable, want: codes.Unavailable},
		{kind: platformmihomo.ErrorInvalidCredential, want: codes.FailedPrecondition},
		{kind: platformmihomo.ErrorExpiredCredential, want: codes.FailedPrecondition},
		{kind: platformmihomo.ErrorChallengeRequired, want: codes.FailedPrecondition},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			err := mapUsecaseError(&platformmihomo.UpstreamError{Kind: test.kind})
			require.Equal(t, test.want, status.Code(err))
			require.NotContains(t, status.Convert(err).Message(), "mihomo upstream request failed")
		})
	}
}
