package clientauth

import (
	"context"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestWithServiceTicketPropagatesOneSanitizedCorrelationSet(t *testing.T) {
	ctx := correlation.Ensure(context.Background(), correlation.Fields{
		RequestID:   "request-outgoing",
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		OperationID: "operation-outgoing",
	})
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"authorization", "Bearer stale",
		correlation.RequestIDHeader, "stale-request",
	))

	ctx = WithServiceTicket(ctx, "fresh-ticket")
	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)
	assert.Equal(t, []string{"Bearer fresh-ticket"}, md.Get("authorization"))
	assert.Equal(t, []string{"request-outgoing"}, md.Get(correlation.RequestIDHeader))
	assert.Equal(t, []string{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}, md.Get(correlation.TraceParentHeader))
	assert.Equal(t, []string{"operation-outgoing"}, md.Get(correlation.OperationIDHeader))
}

func TestWithServiceTicketCreatesCorrelationForBackgroundCalls(t *testing.T) {
	ctx := WithServiceTicket(context.Background(), "ticket")
	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)
	assert.Regexp(t, `^[0-9a-f]{32}$`, md.Get(correlation.RequestIDHeader)[0])
	assert.Regexp(t, `^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`, md.Get(correlation.TraceParentHeader)[0])
	assert.Empty(t, md.Get(correlation.OperationIDHeader))
}
