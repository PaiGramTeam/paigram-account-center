package servicehealth

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorRejectsReadinessAfterShutdown(t *testing.T) {
	checks := 0
	coordinator := NewCoordinator(CheckFunc(func(context.Context) error {
		checks++
		return nil
	}))
	require.NoError(t, coordinator.Check(context.Background()))

	coordinator.Shutdown()
	err := coordinator.Check(context.Background())

	require.ErrorIs(t, err, ErrShuttingDown)
	require.Equal(t, 1, checks)
}

func TestCoordinatorPreservesDependencyFailure(t *testing.T) {
	want := errors.New("redis unavailable")
	coordinator := NewCoordinator(CheckFunc(func(context.Context) error { return want }))

	require.ErrorIs(t, coordinator.Check(context.Background()), want)
}

func TestCoordinatorDoesNotReportReadyWhenShutdownOverlapsCheck(t *testing.T) {
	checkStarted := make(chan struct{})
	releaseCheck := make(chan struct{})
	coordinator := NewCoordinator(CheckFunc(func(context.Context) error {
		close(checkStarted)
		<-releaseCheck
		return nil
	}))

	var wg sync.WaitGroup
	wg.Add(1)
	var checkErr error
	go func() {
		defer wg.Done()
		checkErr = coordinator.Check(context.Background())
	}()
	<-checkStarted
	coordinator.Shutdown()
	close(releaseCheck)
	wg.Wait()

	require.ErrorIs(t, checkErr, ErrShuttingDown)
}
