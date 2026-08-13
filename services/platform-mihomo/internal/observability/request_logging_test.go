package observability

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRequestLoggingMiddlewareWritesCorrelationWithoutPayload(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	ctx := correlation.Ensure(context.Background(), correlation.Fields{
		RequestID:   "request-platform-log",
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	})
	handler := RequestLoggingMiddleware(logger)(func(ctx context.Context, _ any) (any, error) {
		_ = WithVerifiedOperation(ctx, "operation-platform-log")
		return nil, nil
	})

	_, err := handler(ctx, "secret-credential-payload")

	require.NoError(t, err)
	assert.Contains(t, output.String(), "request_id=request-platform-log")
	assert.Contains(t, output.String(), "trace_id=4bf92f3577b34da6a3ce929d0e0e4736")
	assert.Contains(t, output.String(), "operation_id=operation-platform-log")
	assert.NotContains(t, output.String(), "secret-credential-payload")
}

func TestRequestLoggingWrapsAuthenticationRejection(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	reject := func(next middleware.Handler) middleware.Handler {
		return func(context.Context, any) (any, error) {
			return nil, status.Error(codes.Unauthenticated, "secret ticket rejected")
		}
	}
	handler := middleware.Chain(
		CorrelationMiddleware(),
		RequestLoggingMiddleware(logger),
		reject,
	)(func(context.Context, any) (any, error) { return nil, nil })

	_, err := handler(context.Background(), "secret-ticket-payload")

	require.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Contains(t, output.String(), "grpc_code=Unauthenticated")
	assert.NotContains(t, output.String(), "secret")
}

func TestRequestLoggingObservesRecoveredPanicWithoutPayload(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	handler := middleware.Chain(
		CorrelationMiddleware(),
		RequestLoggingMiddleware(logger),
		SanitizedRecoveryMiddleware(logger),
	)(func(context.Context, any) (any, error) {
		panic("secret panic value")
	})

	_, err := handler(context.Background(), "secret-request-payload")

	require.Error(t, err)
	assert.Contains(t, output.String(), "grpc_code=Internal")
	assert.Contains(t, output.String(), "stack=")
	assert.Contains(t, output.String(), "request_logging_test.go")
	assert.NotContains(t, output.String(), "secret")
}
