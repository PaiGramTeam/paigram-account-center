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
)

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

	srv := NewGRPCServer(&conf.Bootstrap{
		Server: &conf.Server{
			Grpc: &conf.Server_GRPC{
				Network:        "tcp",
				Addr:           "127.0.0.1:0",
				TimeoutSeconds: 5,
			},
		},
	}, nil, nil, nil)

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
