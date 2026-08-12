package biz

import (
	"context"
	"errors"
	"time"
)

var ErrCredentialAlreadyBound = errors.New("credential already exists for binding")

type Credential struct {
	BindingID         uint64
	PlatformAccountID string
	Platform          string
	AccountID         string
	Region            string
	CredentialBlob    string
	CredentialVersion string
	Status            string
	LastValidatedAt   *time.Time
	LastRefreshedAt   *time.Time
	ExpiresAt         *time.Time
}

type CredentialRepository interface {
	Create(ctx context.Context, credential *Credential) error
	Save(ctx context.Context, credential *Credential) error
	GetByBindingID(ctx context.Context, bindingID uint64) (*Credential, error)
	GetByPlatformAccountID(ctx context.Context, platformAccountID string) (*Credential, error)
	DeleteByPlatformAccountID(ctx context.Context, platformAccountID string) error
}
