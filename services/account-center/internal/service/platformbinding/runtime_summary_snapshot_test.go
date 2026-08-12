package platformbinding

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"paigram/internal/model"
)

func TestRepairProjectionAppliesCompleteEmptyProfileSnapshot(t *testing.T) {
	binding := &model.PlatformAccountBinding{
		ID: 101, Platform: "mihomo", ExternalAccountKey: sql.NullString{String: "account-101", Valid: true},
	}
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	profiles := &fakeProfileSyncer{}
	service := NewRuntimeSummaryService(&fakeRuntimeSummaryPlatformService{summary: map[string]any{
		"platform_account_id": "account-101", "generation": uint64(4), "status": "active",
		"profiles": []map[string]any{}, "profile_snapshot_complete": true,
		"profile_revision": uint64(4), "profile_observed_revision": uint64(4),
	}}, reader, profiles)

	_, err := service.RepairProjection(context.Background(), binding.ID)
	require.NoError(t, err)
	require.True(t, profiles.called)
	require.Empty(t, profiles.input.Profiles)
	require.Equal(t, uint64(4), profiles.input.Revision)
}

func TestRepairProjectionDoesNotApplyPartialProfileSnapshot(t *testing.T) {
	binding := &model.PlatformAccountBinding{
		ID: 101, Platform: "mihomo", ExternalAccountKey: sql.NullString{String: "account-101", Valid: true},
	}
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	profiles := &fakeProfileSyncer{}
	service := NewRuntimeSummaryService(&fakeRuntimeSummaryPlatformService{summary: map[string]any{
		"platform_account_id": "account-101", "generation": uint64(4), "status": "active",
		"profiles": []map[string]any{{"profile_ref": "profile-new"}}, "profile_snapshot_complete": false,
		"profile_revision": uint64(4), "profile_observed_revision": uint64(3),
	}}, reader, profiles)

	_, err := service.RepairProjection(context.Background(), binding.ID)
	require.NoError(t, err)
	require.False(t, profiles.called)
}
