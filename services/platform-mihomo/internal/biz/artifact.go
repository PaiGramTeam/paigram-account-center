package biz

import (
	"context"
	"time"
)

type Artifact struct {
	BindingRef    string
	AccountKey    string
	ArtifactType  string
	ArtifactValue string
	ScopeKey      string
	ExpiresAt     time.Time
}

type ArtifactRepository interface {
	Put(ctx context.Context, artifact *Artifact) error
	GetByBindingRef(ctx context.Context, bindingRef string, artifactType, scopeKey string) (*Artifact, error)
	Get(ctx context.Context, accountKey, artifactType, scopeKey string) (*Artifact, error)
	DeleteByBindingRef(ctx context.Context, bindingRef string) error
	DeleteByAccountKey(ctx context.Context, accountKey string) error
}
