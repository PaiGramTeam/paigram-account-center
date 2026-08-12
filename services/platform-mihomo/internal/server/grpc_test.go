package server

import (
	"testing"

	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"platform-mihomo-service/internal/service"
)

func TestNewGRPCServersRequiresBothV2Services(t *testing.T) {
	_, err := NewGRPCServers(testSecureBootstrap(t), nil, nil)
	require.EqualError(t, err, "v2 control and runtime services are required")
}

func TestRegisterHealthServerSkipsDuplicateRegistration(t *testing.T) {
	srv := kratosgrpc.NewServer()
	healthServer := health.NewServer()

	registerHealthServer(srv, healthServer)
	registerHealthServer(srv, healthServer)

	if _, ok := srv.GetServiceInfo()[healthpb.Health_ServiceDesc.ServiceName]; !ok {
		t.Fatalf("service %q not registered", healthpb.Health_ServiceDesc.ServiceName)
	}
}

func testV2Services() (*service.PlatformControlService, *service.MihomoRuntimeService) {
	return service.NewPlatformControlService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil),
		service.NewMihomoRuntimeService(nil, nil, nil, nil, nil, nil)
}
