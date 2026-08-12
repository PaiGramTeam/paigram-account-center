package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/servicehealth"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const defaultHealthRefreshInterval = 2 * time.Second

// GRPCHealthCoordinator keeps both gRPC ingress health services aligned with local readiness.
type GRPCHealthCoordinator struct {
	readiness servicehealth.Checker
	interval  time.Duration
	server    *health.Server
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func newGRPCHealthCoordinator(readiness servicehealth.Checker, interval time.Duration) *GRPCHealthCoordinator {
	if interval <= 0 {
		interval = defaultHealthRefreshInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	return &GRPCHealthCoordinator{
		readiness: readiness,
		interval:  interval,
		server:    healthServer,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
}

func (c *GRPCHealthCoordinator) Server() *health.Server {
	return c.server
}

func (c *GRPCHealthCoordinator) Refresh(ctx context.Context) error {
	if c.readiness == nil {
		c.server.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		return errors.New("readiness checker is required")
	}
	if err := c.readiness.Check(ctx); err != nil {
		c.server.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		return err
	}
	c.server.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	return nil
}

func (c *GRPCHealthCoordinator) Start() {
	c.startOnce.Do(func() {
		_ = c.Refresh(c.ctx)
		go c.monitor()
	})
}

func (c *GRPCHealthCoordinator) monitor() {
	defer close(c.done)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			_ = c.Refresh(c.ctx)
		}
	}
}

// Shutdown publishes NOT_SERVING before stopping the background monitor.
func (c *GRPCHealthCoordinator) Shutdown() {
	c.stopOnce.Do(func() {
		c.server.Shutdown()
		c.cancel()
		c.startOnce.Do(func() { close(c.done) })
		<-c.done
	})
}
