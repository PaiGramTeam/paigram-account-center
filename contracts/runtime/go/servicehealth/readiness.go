package servicehealth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const defaultCheckTimeout = 2 * time.Second

// Checker reports whether a service can safely accept new work.
type Checker interface {
	Check(context.Context) error
}

// CheckFunc adapts a function to Checker.
type CheckFunc func(context.Context) error

func (f CheckFunc) Check(ctx context.Context) error {
	return f(ctx)
}

// Dependency names a required local dependency and its readiness check.
type Dependency struct {
	Name  string
	Check CheckFunc
}

// Readiness checks all required local dependencies within one bounded window.
type Readiness struct {
	timeout      time.Duration
	dependencies []Dependency
}

func NewReadiness(timeout time.Duration, dependencies ...Dependency) *Readiness {
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	return &Readiness{timeout: timeout, dependencies: append([]Dependency(nil), dependencies...)}
}

func (r *Readiness) Check(ctx context.Context) error {
	if r == nil {
		return errors.New("readiness checker is required")
	}
	checkCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	var failures []error
	for _, dependency := range r.dependencies {
		if dependency.Check == nil {
			failures = append(failures, fmt.Errorf("%s: readiness check is required", dependencyName(dependency.Name)))
			continue
		}
		if err := dependency.Check(checkCtx); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", dependencyName(dependency.Name), err))
		}
	}
	return errors.Join(failures...)
}

func dependencyName(name string) string {
	if name == "" {
		return "dependency"
	}
	return name
}
