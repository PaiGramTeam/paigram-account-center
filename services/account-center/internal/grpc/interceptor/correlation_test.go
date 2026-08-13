package interceptor

import (
	"context"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestUnaryCorrelationInterceptorAddsValidatedContext(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		correlation.RequestIDHeader, "request-grpc",
		correlation.TraceParentHeader, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		correlation.OperationIDHeader, "operation-grpc",
	))
	var captured correlation.Fields

	_, err := UnaryCorrelationInterceptor()(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ interface{}) (interface{}, error) {
		captured = correlation.FromContext(ctx)
		return nil, nil
	})

	require.NoError(t, err)
	assert.Equal(t, "request-grpc", captured.RequestID)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", captured.TraceID)
	assert.Equal(t, "operation-grpc", captured.OperationID)
}

func TestStreamCorrelationInterceptorWrapsStreamContext(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		correlation.RequestIDHeader, "request-stream",
	))
	stream := &correlationTestServerStream{ctx: ctx}
	var captured correlation.Fields

	err := StreamCorrelationInterceptor()(nil, stream, &grpc.StreamServerInfo{}, func(_ interface{}, stream grpc.ServerStream) error {
		captured = correlation.FromContext(stream.Context())
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, "request-stream", captured.RequestID)
	assert.NotEmpty(t, captured.TraceParent)
}

type correlationTestServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *correlationTestServerStream) Context() context.Context {
	return s.ctx
}
