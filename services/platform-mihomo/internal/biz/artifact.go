package biz

import (
	"context"
	"time"
)

type Artifact struct {
	BindingID         uint64
	PlatformAccountID string
	ArtifactType      string
	ArtifactValue     string
	ScopeKey          string
	ExpiresAt         time.Time
}

type ArtifactRepository interface {
	Put(ctx context.Context, artifact *Artifact) error
	GetByBindingID(ctx context.Context, bindingID uint64, artifactType, scopeKey string) (*Artifact, error)
	Get(ctx context.Context, platformAccountID, artifactType, scopeKey string) (*Artifact, error)
	DeleteByBindingID(ctx context.Context, bindingID uint64) error
	DeleteByPlatformAccountID(ctx context.Context, platformAccountID string) error
}
