package server

import (
	"context"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/servicehealth"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	defaultHealthRefreshInterval = 2 * time.Second
	livenessServiceName          = "liveness"
)

// GRPCHealthCoordinator projects shared readiness onto both gRPC ingress health services.
type GRPCHealthCoordinator struct {
	server  *health.Server
	monitor *servicehealth.Monitor
}

func newGRPCHealthCoordinator(readiness servicehealth.Checker, interval time.Duration) *GRPCHealthCoordinator {
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	healthServer.SetServingStatus(livenessServiceName, healthpb.HealthCheckResponse_SERVING)
	return &GRPCHealthCoordinator{
		server:  healthServer,
		monitor: servicehealth.NewMonitor(readiness, &grpcHealthPublisher{server: healthServer}, interval),
	}
}

func (c *GRPCHealthCoordinator) Server() *health.Server {
	return c.server
}

func (c *GRPCHealthCoordinator) Refresh(ctx context.Context) error {
	return c.monitor.Refresh(ctx)
}

func (c *GRPCHealthCoordinator) Start() {
	c.monitor.Start()
}

// Shutdown publishes NOT_SERVING before stopping the background monitor.
func (c *GRPCHealthCoordinator) Shutdown() {
	c.monitor.Shutdown()
}

type grpcHealthPublisher struct {
	server *health.Server
}

func (p *grpcHealthPublisher) SetReady(ready bool) {
	status := healthpb.HealthCheckResponse_NOT_SERVING
	if ready {
		status = healthpb.HealthCheckResponse_SERVING
	}
	p.server.SetServingStatus("", status)
}

func (p *grpcHealthPublisher) Shutdown() {
	p.server.Shutdown()
}
