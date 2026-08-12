package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

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
