package biz

import (
	"context"
	"errors"
	"time"
)

var ErrCredentialAlreadyBound = errors.New("credential already exists for binding")
var ErrCredentialGenerationConflict = errors.New("credential generation conflict")

type Credential struct {
	BindingRef              string
	AccountKey              string
	Generation              uint64
	Platform                string
	AccountID               string
	Region                  string
	CredentialBlob          string
	CredentialVersion       string
	Status                  string
	LastValidatedAt         *time.Time
	LastRefreshedAt         *time.Time
	ExpiresAt               *time.Time
	ProfileSnapshotComplete bool
	ProfileRevision         uint64
	ProfileObservedRevision uint64
}

type CredentialRepository interface {
	Create(ctx context.Context, credential *Credential) error
	Save(ctx context.Context, credential *Credential) error
	AdvanceGeneration(ctx context.Context, bindingRef, accountKey string, expected, target uint64) (*Credential, error)
	AdvanceProfileRevision(ctx context.Context, bindingRef, accountKey string, generation, expectedRevision uint64) (*Credential, error)
	SetProfileSnapshotState(ctx context.Context, bindingRef string, complete bool, revision, observedRevision uint64) error
	GetByBindingRef(ctx context.Context, bindingRef string) (*Credential, error)
	GetByBindingRefForUpdate(ctx context.Context, bindingRef string) (*Credential, error)
	GetByAccountKey(ctx context.Context, accountKey string) (*Credential, error)
	DeleteByAccountKey(ctx context.Context, accountKey string) error
}
