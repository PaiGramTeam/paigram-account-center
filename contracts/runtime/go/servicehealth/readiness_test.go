package servicehealth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReadinessFailsWhenRequiredDependencyFails(t *testing.T) {
	want := errors.New("database unavailable")
	readiness := NewReadiness(time.Second,
		Dependency{Name: "database", Check: func(context.Context) error { return want }},
		Dependency{Name: "redis", Check: func(context.Context) error { return nil }},
	)

	err := readiness.Check(context.Background())

	require.ErrorIs(t, err, want)
	require.ErrorContains(t, err, "database")
}

func TestReadinessBoundsDependencyChecks(t *testing.T) {
	readiness := NewReadiness(10*time.Millisecond, Dependency{
		Name: "database",
		Check: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})

	err := readiness.Check(context.Background())

	require.ErrorIs(t, err, context.DeadlineExceeded)
}
