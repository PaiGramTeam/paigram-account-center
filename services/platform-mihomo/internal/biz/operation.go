package biz

import (
	"context"
	"errors"
	"time"
)

var ErrOperationConflict = errors.New("operation id is already bound to different input")
var ErrOperationState = errors.New("operation result has an invalid state transition")

type OperationRef struct {
	OperationID        string
	Kind               string
	BindingRef         string
	PreGeneration      uint64
	TargetGeneration   uint64
	RequestFingerprint string
}

type OperationResult struct {
	Operation      OperationRef
	State          string
	ReasonCode     string
	AccountKey     string
	Status         string
	SnapshotJSON   string
	ExecutionToken string
	LeaseExpiresAt time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type OperationRepository interface {
	WithinTransaction(ctx context.Context, fn func(context.Context) error) error
	Admit(ctx context.Context, operation OperationRef) (*OperationResult, bool, error)
	LockPending(ctx context.Context, operationID, executionToken string) error
	Complete(ctx context.Context, result OperationResult) error
	FailPending(ctx context.Context, operationID, executionToken, reasonCode string) error
	Get(ctx context.Context, operationID string) (*OperationResult, error)
	Resolve(ctx context.Context, operation OperationRef) (*OperationResult, error)
}
