package biz

import (
	"context"
	"errors"
	"time"
)

var ErrArtifactCredentialStale = errors.New("artifact credential state changed")
var ErrArtifactRevocationPending = errors.New("artifact revocation is pending")

type Artifact struct {
	BindingRef    string
	AccountKey    string
	ArtifactType  string
	ArtifactValue string
	ScopeKey      string
	ExpiresAt     time.Time
}

type ArtifactRevocationIntent struct {
	IntentID       string
	BindingRef     string
	AccountKey     string
	ArtifactType   string
	ArtifactValue  string
	ScopeKey       string
	ExpiresAt      time.Time
	State          string
	ReadyAfter     time.Time
	LeaseToken     string
	LeaseExpiresAt *time.Time
}

type ArtifactRepository interface {
	Put(ctx context.Context, artifact *Artifact) error
	PutIfCredentialCurrent(ctx context.Context, artifact *Artifact, expectedGeneration uint64) error
	ListByBindingRef(ctx context.Context, bindingRef string) ([]*Artifact, error)
	HasRevocationPending(ctx context.Context, bindingRef string) (bool, error)
	PutRevocationIntentImmediately(ctx context.Context, intent *ArtifactRevocationIntent) (*ArtifactRevocationIntent, error)
	MarkRevocationIntentReadyImmediately(ctx context.Context, intentID string) error
	ClaimRevocationIntents(ctx context.Context, now, leaseExpiresAt time.Time, leaseToken string) ([]*ArtifactRevocationIntent, error)
	ResolveProvisionalRevocationIntent(ctx context.Context, intentID, leaseToken string) (bool, error)
	ReleaseRevocationIntentClaim(ctx context.Context, intentID, leaseToken string) error
	FinalizeRevocationIntentImmediately(ctx context.Context, intentID string) error
	DeleteRevocationIntentImmediately(ctx context.Context, intentID string) error
	GetByBindingRef(ctx context.Context, bindingRef string, artifactType, scopeKey string) (*Artifact, error)
	Get(ctx context.Context, accountKey, artifactType, scopeKey string) (*Artifact, error)
	DeleteByBindingRef(ctx context.Context, bindingRef string) error
	DeleteByBindingRefImmediately(ctx context.Context, bindingRef string) error
	DeleteArtifactImmediately(ctx context.Context, bindingRef, artifactType, scopeKey, artifactValue string) error
	MarkRevocationPendingImmediately(ctx context.Context, bindingRef string) error
	DeleteByAccountKey(ctx context.Context, accountKey string) error
	DeleteExpired(ctx context.Context, expiredBefore time.Time) (int64, error)
}
