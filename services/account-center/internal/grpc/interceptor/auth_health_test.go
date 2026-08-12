package interceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestHealthCheckDoesNotRequireOAuthToken(t *testing.T) {
	interceptor := NewAuthInterceptor(nil)
	handled := false

	_, err := interceptor.Unary()(context.Background(), nil, &grpc.UnaryServerInfo{
		FullMethod: healthpb.Health_Check_FullMethodName,
	}, func(context.Context, interface{}) (interface{}, error) {
		handled = true
		return nil, nil
	})

	require.NoError(t, err)
	require.True(t, handled)
}
