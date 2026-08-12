package server

import (
	"context"
	"testing"
	"time"

	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"platform-mihomo-service/internal/conf"
	"platform-mihomo-service/internal/service"
)

func TestNewGRPCServerRegistersOnlyV2PlatformAPIs(t *testing.T) {
	control, runtime := testV2Services()
	srv := NewGRPCServer(testBootstrap(), control, runtime)

	services := srv.GetServiceInfo()
	for _, legacyService := range []string{"mihomo.v1.MihomoAccountService", "paigram.mihomo.v1.MihomoCredentialService", "paigram.platform.v1.PlatformService"} {
		if _, ok := services[legacyService]; ok {
			t.Fatalf("legacy service %q is registered", legacyService)
		}
	}
	for _, v2Service := range []string{"paigram.platform.v2.PlatformControlService", "paigram.mihomo.v2.MihomoRuntimeService"} {
		if _, ok := services[v2Service]; !ok {
			t.Fatalf("v2 service %q is not registered", v2Service)
		}
	}
}

func TestNewGRPCServerRequiresBothV2Services(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != "v2 control and runtime services are required" {
			t.Fatalf("panic = %v, want v2 control and runtime services are required", recovered)
		}
	}()

	NewGRPCServer(testBootstrap(), nil, nil)
}

func TestRegisterHealthServerSkipsDuplicateRegistration(t *testing.T) {
	srv := kratosgrpc.NewServer()

	registerHealthServer(srv)
	registerHealthServer(srv)

	if _, ok := srv.GetServiceInfo()[healthpb.Health_ServiceDesc.ServiceName]; !ok {
		t.Fatalf("service %q not registered", healthpb.Health_ServiceDesc.ServiceName)
	}
}

func TestNewGRPCServerExposesHealth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	control, runtime := testV2Services()
	srv := NewGRPCServer(testBootstrap(), control, runtime)

	endpoint, err := srv.Endpoint()
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Start(context.Background())
	}()
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := srv.Stop(stopCtx); err != nil {
			t.Fatalf("stop: %v", err)
		}
		if err := <-serveErr; err != nil && err != grpc.ErrServerStopped {
			t.Fatalf("start: %v", err)
		}
	})

	conn, err := grpc.DialContext(
		ctx,
		endpoint.Host,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("check health: %v", err)
	}

	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("health status = %s, want %s", resp.GetStatus(), healthpb.HealthCheckResponse_SERVING)
	}
}

func testV2Services() (*service.PlatformControlService, *service.MihomoRuntimeService) {
	return service.NewPlatformControlService(nil, nil, nil, nil, nil, nil, nil, nil, nil),
		service.NewMihomoRuntimeService(nil, nil, nil, nil, nil, nil)
}

func testBootstrap() *conf.Bootstrap {
	return &conf.Bootstrap{
		Server: &conf.Server{
			Grpc: &conf.Server_GRPC{
				Network:        "tcp",
				Addr:           "127.0.0.1:0",
				TimeoutSeconds: 5,
			},
		},
	}
}
