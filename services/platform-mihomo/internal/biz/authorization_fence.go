package biz

import "context"

type AuthorizationFence struct {
	BindingRef           string
	ConsumerPrincipal    string
	MinimumGrantVersion  uint64
	MinimumOwnerEpoch    uint64
	MinimumConsumerEpoch uint64
	MinimumEntryEpoch    uint64
}

type AuthorizationFenceRepository interface {
	Upsert(ctx context.Context, fence AuthorizationFence) error
}
