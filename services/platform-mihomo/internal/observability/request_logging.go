package observability

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type operationCaptureKey struct{}

type operationCapture struct {
	mu          sync.RWMutex
	operationID string
}

func RequestLoggingMiddleware(logger *slog.Logger) middleware.Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, request any) (any, error) {
			startedAt := time.Now()
			capture := &operationCapture{}
			ctx = context.WithValue(ctx, operationCaptureKey{}, capture)
			response, err := next(ctx, request)
			fields := correlation.FromContext(ctx)
			attributes := []any{
				"request_id", fields.RequestID,
				"trace_id", fields.TraceID,
				"grpc_code", status.Code(err).String(),
				"duration_ms", time.Since(startedAt).Milliseconds(),
			}
			if operationID := capture.get(); operationID != "" {
				attributes = append(attributes, "operation_id", operationID)
			}
			if serverTransport, ok := transport.FromServerContext(ctx); ok {
				attributes = append(attributes, "rpc", serverTransport.Operation())
			}
			if err != nil {
				logger.ErrorContext(ctx, "grpc request completed", attributes...)
			} else {
				logger.InfoContext(ctx, "grpc request completed", attributes...)
			}
			return response, err
		}
	}
}

func WithVerifiedOperation(ctx context.Context, operationID string) context.Context {
	ctx = correlation.WithOperationID(ctx, operationID)
	if capture, ok := ctx.Value(operationCaptureKey{}).(*operationCapture); ok && capture != nil {
		capture.set(correlation.FromContext(ctx).OperationID)
	}
	return ctx
}

func SanitizedRecoveryMiddleware(logger *slog.Logger) middleware.Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, request any) (response any, err error) {
			defer func() {
				if recover() == nil {
					return
				}
				fields := correlation.FromContext(ctx)
				stack := make([]byte, 64<<10)
				stack = stack[:runtime.Stack(stack, false)]
				logger.ErrorContext(ctx, "grpc panic recovered", "request_id", fields.RequestID, "trace_id", fields.TraceID, "stack", string(stack))
				response = nil
				err = status.Error(codes.Internal, "internal server error")
			}()
			return next(ctx, request)
		}
	}
}

func (c *operationCapture) set(operationID string) {
	c.mu.Lock()
	c.operationID = operationID
	c.mu.Unlock()
}

func (c *operationCapture) get() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.operationID
}
