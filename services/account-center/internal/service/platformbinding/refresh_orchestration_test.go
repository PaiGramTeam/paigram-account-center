package platformbinding

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/model"
)

func TestRefreshBindingProjectsAuthoritativeProfileSnapshotBeforeCompletingIntent(t *testing.T) {
	binding := &model.PlatformAccountBinding{
		ID: 101, OwnerUserID: 7, Platform: "mihomo", PlatformServiceKey: "platform-mihomo-service",
		BindingRef: "bind_test", Generation: 4, ProfileRevision: 7,
		ExternalAccountKey: sql.NullString{String: "account-101", Valid: true},
		Status:             model.PlatformAccountBindingStatusRefreshRequired,
	}
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	platformService := &fakeOrchestrationPlatformService{
		platform: &model.PlatformService{ControlEndpoint: "127.0.0.1:9000"},
		ticket:   "service-ticket",
	}
	gateway := &fakeRefreshGateway{summary: &RuntimeSummary{
		PlatformAccountID: "account-101", Generation: 5, Status: "active",
		ProfileSnapshotComplete: true, ProfileRevision: 8, ProfileObservedRevision: 8,
		Profiles: []map[string]any{{
			"profile_ref": "profile-stable", "game_biz": "hk4e_cn", "region": "cn_gf01",
			"player_id": "1008611", "nickname": "Traveler", "is_default": true,
		}},
	}}
	profileSyncer := &fakeProfileSyncer{}
	store := NewOperationIntentService(openOperationIntentTestDB(t))
	service := NewOrchestrationService(reader, platformService, gateway, profileSyncer, store)

	updated, err := service.RefreshBindingForOwner(context.Background(), 7, 101)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.True(t, profileSyncer.called)
	assert.Equal(t, uint64(8), profileSyncer.input.Revision)
	require.Len(t, profileSyncer.input.Profiles, 1)
	assert.Equal(t, "profile-stable", profileSyncer.input.Profiles[0].ProfileRef)

	var intent model.PlatformOperationIntent
	require.NoError(t, store.db.Take(&intent).Error)
	assert.Equal(t, "OPERATION_KIND_REFRESH_CREDENTIAL", intent.Kind)
	assert.Equal(t, model.PlatformOperationIntentStateSucceeded, intent.State)
}

func TestRefreshBindingPersistsIncompleteSnapshotWithoutDeletingProfiles(t *testing.T) {
	binding := &model.PlatformAccountBinding{
		ID: 101, OwnerUserID: 7, Platform: "mihomo", PlatformServiceKey: "platform-mihomo-service",
		BindingRef: "bind_test", Generation: 4, ProfileRevision: 7, ProfileObservedRevision: 7,
		ExternalAccountKey: sql.NullString{String: "account-101", Valid: true},
		Status:             model.PlatformAccountBindingStatusActive,
	}
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	platformService := &fakeOrchestrationPlatformService{
		platform: &model.PlatformService{ControlEndpoint: "127.0.0.1:9000"},
		ticket:   "service-ticket",
	}
	gateway := &fakeRefreshGateway{summary: &RuntimeSummary{
		PlatformAccountID: "account-101", Generation: 5, Status: "credential_invalid",
		ProfileSnapshotComplete: false, ProfileRevision: 7, ProfileObservedRevision: 7,
	}}
	profileSyncer := &fakeProfileSyncer{}
	store := NewOperationIntentService(openOperationIntentTestDB(t))
	service := NewOrchestrationService(reader, platformService, gateway, profileSyncer, store)

	updated, err := service.RefreshBindingForOwner(context.Background(), 7, 101)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.False(t, profileSyncer.called)
	require.NotNil(t, reader.persistedSummary)
	assert.False(t, reader.persistedSummary.ProfileSnapshotComplete)

	var intent model.PlatformOperationIntent
	require.NoError(t, store.db.Take(&intent).Error)
	assert.Equal(t, model.PlatformOperationIntentStateSucceeded, intent.State)
}
