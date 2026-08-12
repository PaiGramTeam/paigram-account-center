package servicehealth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitorPublishesReadinessAndShutdown(t *testing.T) {
	dependency := &monitorReadiness{}
	publisher := &recordingPublisher{}
	monitor := NewMonitor(dependency, publisher, time.Hour)

	require.NoError(t, monitor.Refresh(context.Background()))
	require.True(t, publisher.Ready())

	dependency.Set(errors.New("database unavailable"))
	require.Error(t, monitor.Refresh(context.Background()))
	require.False(t, publisher.Ready())

	monitor.Shutdown()
	require.True(t, publisher.ShutdownCalled())
}

type monitorReadiness struct {
	mu  sync.RWMutex
	err error
}

func (r *monitorReadiness) Check(context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.err
}

func (r *monitorReadiness) Set(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

type recordingPublisher struct {
	mu       sync.RWMutex
	ready    bool
	shutdown bool
}

func (p *recordingPublisher) SetReady(ready bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ready = ready
}

func (p *recordingPublisher) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shutdown = true
	p.ready = false
}

func (p *recordingPublisher) Ready() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ready
}

func (p *recordingPublisher) ShutdownCalled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.shutdown
}
