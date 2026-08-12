//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
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

func TestConcurrentRuntimeSnapshotsRemainMonotonic(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	bindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "account-concurrent-snapshot")
	_, err := stack.SQLDB.ExecContext(ctx, `
		UPDATE platform_account_bindings
		SET generation = 3, profile_revision = 3, profile_observed_revision = 3
		WHERE id = $1
	`, bindingID)
	require.NoError(t, err)

	blocker, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Rollback() })
	_, err = blocker.ExecContext(ctx, `SELECT id FROM platform_account_bindings WHERE id = $1 FOR UPDATE`, bindingID)
	require.NoError(t, err)

	bindingService := serviceplatformbinding.NewBindingService(stack.DB)
	projectionService := serviceplatformbinding.NewProfileProjectionService(stack.DB)
	type snapshotCase struct {
		generation uint64
		status     string
		profileKey string
	}
	cases := []snapshotCase{
		{generation: 4, status: "invalid", profileKey: "profile-old"},
		{generation: 5, status: "active", profileKey: "profile-new"},
	}
	start := make(chan struct{})
	errorsByGeneration := make(map[uint64]error, len(cases))
	var resultMu sync.Mutex
	var wait sync.WaitGroup
	for _, testCase := range cases {
		testCase := testCase
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, flowErr := bindingService.PersistRuntimeSummary(bindingID, serviceplatformbinding.RuntimeSummary{
				Generation: testCase.generation, Status: testCase.status,
				ProfileRevision: testCase.generation, ProfileObservedRevision: testCase.generation,
			})
			if flowErr == nil {
				_, flowErr = projectionService.SyncProfiles(serviceplatformbinding.SyncProfilesInput{
					BindingID: bindingID, Revision: testCase.generation, ObservedRevision: testCase.generation,
					SyncedAt: time.Now().UTC(), Profiles: []serviceplatformbinding.ProfileProjectionInput{{
						PlatformProfileKey: testCase.profileKey, ProfileRef: testCase.profileKey + "-ref",
						GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerUID: fmt.Sprintf("1000%d", testCase.generation),
						Nickname: testCase.profileKey, IsPrimary: true,
					}},
				})
			}
			resultMu.Lock()
			errorsByGeneration[testCase.generation] = flowErr
			resultMu.Unlock()
		}()
	}
	close(start)
	waitForPostgresLockWaiters(t, ctx, stack.SQLDB, "platform_account_bindings", 2)
	require.NoError(t, blocker.Commit())
	wait.Wait()
	require.NoError(t, errorsByGeneration[4])
	require.NoError(t, errorsByGeneration[5])

	var generation, revision, observedRevision uint64
	var status string
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `
		SELECT generation, profile_revision, profile_observed_revision, status
		FROM platform_account_bindings WHERE id = $1
	`, bindingID).Scan(&generation, &revision, &observedRevision, &status))
	require.Equal(t, uint64(5), generation)
	require.Equal(t, uint64(5), revision)
	require.Equal(t, uint64(5), observedRevision)
	require.Equal(t, "active", status)

	var profileKey string
	var profileCount int
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(MAX(platform_profile_key), '')
		FROM platform_account_profiles WHERE binding_id = $1
	`, bindingID).Scan(&profileCount, &profileKey))
	require.Equal(t, 1, profileCount)
	require.Equal(t, "profile-new", profileKey)
}

func waitForPostgresLockWaiters(t *testing.T, ctx context.Context, db *sql.DB, relation string, minimum int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var count int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM pg_stat_activity
			WHERE datname = current_database()
			  AND pid <> pg_backend_pid()
			  AND wait_event_type = 'Lock'
			  AND query ILIKE '%' || $1 || '%'
		`, relation).Scan(&count)
		return err == nil && count >= minimum
	}, 5*time.Second, 10*time.Millisecond)
}
