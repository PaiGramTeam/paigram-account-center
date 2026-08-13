package platformbinding

import (
	"context"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestCredentialGatewayCallContextPropagatesOperationID(t *testing.T) {
	ctx := correlation.Ensure(context.Background(), correlation.Fields{RequestID: "request-gateway"})
	callCtx, cancel := credentialGatewayCallContext(ctx, "ticket", "operation-gateway")
	defer cancel()

	md, ok := metadata.FromOutgoingContext(callCtx)
	require.True(t, ok)
	assert.Equal(t, []string{"request-gateway"}, md.Get(correlation.RequestIDHeader))
	assert.Equal(t, []string{"operation-gateway"}, md.Get(correlation.OperationIDHeader))
}
