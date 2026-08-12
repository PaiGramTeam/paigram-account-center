//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	serviceplatformbinding "paigram/internal/service/platformbinding"
)

func TestProfileProjectionSwitchesForeignKeyBeforeDeletingStalePrimary(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	bindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "account-primary-repair")
	originalPrimaryID := insertTestProfile(t, ctx, stack.SQLDB, bindingID, "profile-original", true)
	_, err := stack.SQLDB.ExecContext(ctx, `UPDATE platform_account_bindings SET primary_profile_id = $1 WHERE id = $2`, originalPrimaryID, bindingID)
	require.NoError(t, err)

	service := serviceplatformbinding.NewProfileProjectionService(stack.DB)
	profiles, err := service.SyncProfiles(serviceplatformbinding.SyncProfilesInput{
		BindingID: bindingID, Revision: 1, ObservedRevision: 1, SyncedAt: time.Now().UTC(),
		Profiles: []serviceplatformbinding.ProfileProjectionInput{{
			PlatformProfileKey: "profile-replacement", ProfileRef: "profile-replacement-ref",
			GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerUID: "10002", Nickname: "Replacement", IsPrimary: true,
		}},
	})
	require.NoError(t, err)
	require.Len(t, profiles, 1)

	var primaryProfileID sql.NullInt64
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT primary_profile_id FROM platform_account_bindings WHERE id = $1`, bindingID).Scan(&primaryProfileID))
	require.Equal(t, sql.NullInt64{Int64: int64(profiles[0].ID), Valid: true}, primaryProfileID)
	var originalCount int
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_account_profiles WHERE id = $1`, originalPrimaryID).Scan(&originalCount))
	require.Zero(t, originalCount)

	profiles, err = service.SyncProfiles(serviceplatformbinding.SyncProfilesInput{
		BindingID: bindingID, Revision: 2, ObservedRevision: 2, SyncedAt: time.Now().UTC(), Profiles: []serviceplatformbinding.ProfileProjectionInput{},
	})
	require.NoError(t, err)
	require.Empty(t, profiles)
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT primary_profile_id FROM platform_account_bindings WHERE id = $1`, bindingID).Scan(&primaryProfileID))
	require.False(t, primaryProfileID.Valid)
	var remainingCount int
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_account_profiles WHERE binding_id = $1`, bindingID).Scan(&remainingCount))
	require.Zero(t, remainingCount)
}
