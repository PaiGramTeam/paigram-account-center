package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"platform-mihomo-service/internal/data/model"
)

type TicketAuthorizationStateLookup struct {
	db *gorm.DB
}

func NewTicketAuthorizationStateLookup(db *gorm.DB) *TicketAuthorizationStateLookup {
	return &TicketAuthorizationStateLookup{db: db}
}

func (l *TicketAuthorizationStateLookup) LookupAuthorizationState(ctx context.Context, bindingRef, consumer string) (AuthorizationState, error) {
	var credential model.CredentialRecord
	if err := l.db.WithContext(ctx).Select("generation").Where("binding_ref = ?", bindingRef).Take(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AuthorizationState{}, nil
		}
		return AuthorizationState{}, err
	}

	state := AuthorizationState{CredentialGeneration: credential.Generation}
	var fence model.AuthorizationFence
	err := l.db.WithContext(ctx).Where("binding_ref = ? AND consumer_principal = ?", bindingRef, consumer).Take(&fence).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return state, nil
	}
	if err != nil {
		return AuthorizationState{}, err
	}
	state.MinimumGrantVersion = fence.MinimumGrantVersion
	state.MinimumOwnerEpoch = fence.MinimumOwnerEpoch
	state.MinimumConsumerEpoch = fence.MinimumConsumerEpoch
	state.MinimumEntryEpoch = fence.MinimumEntryEpoch
	return state, nil
}
