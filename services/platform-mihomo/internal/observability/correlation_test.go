package observability

import (
	"context"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestCorrelationMiddlewareAddsValidatedContext(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		correlation.RequestIDHeader, "request-platform",
		correlation.TraceParentHeader, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		correlation.OperationIDHeader, "untrusted-operation-platform",
	))
	var captured correlation.Fields
	handler := CorrelationMiddleware()(func(ctx context.Context, _ any) (any, error) {
		captured = correlation.FromContext(ctx)
		return nil, nil
	})

	_, err := handler(ctx, nil)

	require.NoError(t, err)
	assert.Equal(t, "request-platform", captured.RequestID)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", captured.TraceID)
	assert.Empty(t, captured.OperationID)
}

func TestCorrelationMiddlewareIsFirstInMiddlewareChain(t *testing.T) {
	seen := false
	outer := CorrelationMiddleware()
	inner := func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, request any) (any, error) {
			seen = correlation.FromContext(ctx).RequestID != ""
			return next(ctx, request)
		}
	}

	_, err := middleware.Chain(outer, inner)(func(context.Context, any) (any, error) {
		return nil, nil
	})(context.Background(), nil)

	require.NoError(t, err)
	assert.True(t, seen)
}
