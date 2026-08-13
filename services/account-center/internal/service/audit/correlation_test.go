package audit

import (
	"context"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/metadata"
)

func TestRequestIDFromContextPrefersValidatedCorrelation(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		correlation.RequestIDHeader, "untrusted-metadata",
	))
	ctx = correlation.Ensure(ctx, correlation.Fields{RequestID: "validated-request"})

	assert.Equal(t, "validated-request", requestIDFromContext(ctx))
}

func TestRequestIDFromContextDoesNotReturnInvalidMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		correlation.RequestIDHeader, "request\r\ninjected",
	))

	assert.Regexp(t, `^[0-9a-f]{32}$`, requestIDFromContext(ctx))
}
