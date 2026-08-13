package interceptor

import (
	"context"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func UnaryCorrelationInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(correlationContext(ctx), request)
	}
}

func StreamCorrelationInterceptor() grpc.StreamServerInterceptor {
	return func(server interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(server, &correlationServerStream{
			ServerStream: stream,
			ctx:          correlationContext(stream.Context()),
		})
	}
}

type correlationServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *correlationServerStream) Context() context.Context {
	return s.ctx
}

func correlationContext(ctx context.Context) context.Context {
	md, _ := metadata.FromIncomingContext(ctx)
	return correlation.Ensure(ctx, correlation.IncomingFields(
		md.Get(correlation.RequestIDHeader),
		md.Get(correlation.TraceParentHeader),
		md.Get(correlation.OperationIDHeader),
	))
}
