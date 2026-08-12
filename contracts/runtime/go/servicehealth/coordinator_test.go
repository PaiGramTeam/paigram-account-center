package servicehealth

import (
	"context"
	"errors"
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
