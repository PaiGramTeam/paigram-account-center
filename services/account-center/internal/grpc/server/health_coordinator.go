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

type grpcHealthCoordinator struct {
	server  *health.Server
	monitor *servicehealth.Monitor
}

func newGRPCHealthCoordinator(readiness servicehealth.Checker, interval time.Duration) *grpcHealthCoordinator {
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	healthServer.SetServingStatus(livenessServiceName, healthpb.HealthCheckResponse_SERVING)
	return &grpcHealthCoordinator{
		server:  healthServer,
		monitor: servicehealth.NewMonitor(readiness, &grpcHealthPublisher{server: healthServer}, interval),
	}
}

func (c *grpcHealthCoordinator) Server() *health.Server {
	return c.server
}

func (c *grpcHealthCoordinator) Refresh(ctx context.Context) error {
	return c.monitor.Refresh(ctx)
}

func (c *grpcHealthCoordinator) Start() {
	c.monitor.Start()
}

func (c *grpcHealthCoordinator) Shutdown() {
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
