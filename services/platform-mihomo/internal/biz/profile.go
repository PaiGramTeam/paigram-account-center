package biz

import (
	"context"
	"time"
)

type ProfileIdentity struct {
	PlayerID string
	Region   string
}

type Profile struct {
	ID           uint64
	BindingRef   string
	AccountKey   string
	ProfileRef   string
	GameBiz      string
	Region       string
	PlayerID     string
	Nickname     string
	Level        int
	IsDefault    bool
	DiscoveredAt time.Time
}

type ProfileRepository interface {
	Save(ctx context.Context, profile *Profile) error
	SetDefaultByBindingAndPlayerID(ctx context.Context, bindingRef string, accountKey string, playerID string) error
	GetByProfileRef(ctx context.Context, bindingRef string, profileRef string) (*Profile, error)
	ListByBindingRef(ctx context.Context, bindingRef string) ([]*Profile, error)
	ListByAccountKey(ctx context.Context, accountKey string) ([]*Profile, error)
	DeleteMissingByBindingRef(ctx context.Context, bindingRef string, keep []ProfileIdentity) error
	DeleteByAccountKey(ctx context.Context, accountKey string) error
	DeleteMissingByAccountKey(ctx context.Context, accountKey string, keep []ProfileIdentity) error
}
