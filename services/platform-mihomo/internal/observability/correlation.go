package observability

import (
	"context"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/go-kratos/kratos/v2/middleware"
	"google.golang.org/grpc/metadata"
)

func CorrelationMiddleware() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, request any) (any, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			return next(correlation.Ensure(ctx, correlation.IncomingFields(
				md.Get(correlation.RequestIDHeader),
				md.Get(correlation.TraceParentHeader),
				nil,
			)), request)
		}
	}
}
