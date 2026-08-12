package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/servicehealth"
	"github.com/stretchr/testify/require"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestGRPCHealthCoordinatorTracksReadinessAndShutdown(t *testing.T) {
	dependency := &mutableReadiness{}
	coordinator := newGRPCHealthCoordinator(dependency, time.Hour)

	require.NoError(t, coordinator.Refresh(context.Background()))
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, platformCoordinatorStatus(t, coordinator))

	dependency.Set(errors.New("redis unavailable"))
	require.Error(t, coordinator.Refresh(context.Background()))
	require.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, platformCoordinatorStatus(t, coordinator))

	dependency.Set(nil)
	require.NoError(t, coordinator.Refresh(context.Background()))
	coordinator.Shutdown()
	require.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, platformCoordinatorStatus(t, coordinator))
}

func platformCoordinatorStatus(t *testing.T, coordinator *GRPCHealthCoordinator) healthpb.HealthCheckResponse_ServingStatus {
	t.Helper()
	response, err := coordinator.Server().Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	return response.GetStatus()
}

type mutableReadiness struct {
	mu  sync.RWMutex
	err error
}

var _ servicehealth.Checker = (*mutableReadiness)(nil)

func (r *mutableReadiness) Check(context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.err
}

func (r *mutableReadiness) Set(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}
