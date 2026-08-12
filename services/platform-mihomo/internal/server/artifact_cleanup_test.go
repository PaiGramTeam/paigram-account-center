package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestArtifactCleanupServerPurgesExpiredRowsUntilStopped(t *testing.T) {
	cleaner := &recordingArtifactCleaner{called: make(chan struct{}, 2)}
	worker := NewArtifactCleanupServer(cleaner, 5*time.Millisecond)
	done := make(chan error, 1)
	go func() {
		done <- worker.Start(context.Background())
	}()

	require.Eventually(t, func() bool { return cleaner.callCount() >= 2 }, time.Second, 5*time.Millisecond)
	require.NoError(t, worker.Stop(context.Background()))
	require.NoError(t, <-done)
}

type recordingArtifactCleaner struct {
	mu     sync.Mutex
	calls  int
	called chan struct{}
}

func (c *recordingArtifactCleaner) DeleteExpired(context.Context, time.Time) (int64, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	select {
	case c.called <- struct{}{}:
	default:
	}
	return 0, nil
}

func (c *recordingArtifactCleaner) RetryPending(context.Context) error {
	return nil
}

func (c *recordingArtifactCleaner) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}
