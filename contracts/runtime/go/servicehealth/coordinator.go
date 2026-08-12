package servicehealth

import (
	"context"
	"errors"
	"sync/atomic"
)

var ErrShuttingDown = errors.New("service is shutting down")

// Coordinator adds an irreversible shutdown gate to a readiness checker.
type Coordinator struct {
	readiness    Checker
	shuttingDown atomic.Bool
}

func NewCoordinator(readiness Checker) *Coordinator {
	return &Coordinator{readiness: readiness}
}

func (c *Coordinator) Check(ctx context.Context) error {
	if c == nil || c.shuttingDown.Load() {
		return ErrShuttingDown
	}
	if c.readiness == nil {
		return errors.New("readiness checker is required")
	}
	return c.readiness.Check(ctx)
}

// Shutdown makes future readiness checks fail before transport draining begins.
func (c *Coordinator) Shutdown() {
	if c != nil {
		c.shuttingDown.Store(true)
	}
}
