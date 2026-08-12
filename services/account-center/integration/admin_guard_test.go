//go:build integration

package integration

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"paigram/internal/model"
)

func TestActiveAdministratorGuardSerializesDestructiveMutations(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx := context.Background()
	adminRoleID := ensureGuardAdminRole(t, stack)
	firstAdminID := insertActiveGuardUser(t, ctx, stack.SQLDB)
	secondAdminID := insertActiveGuardUser(t, ctx, stack.SQLDB)
	insertGuardAdminAssignment(t, ctx, stack.SQLDB, firstAdminID, adminRoleID)
	insertGuardAdminAssignment(t, ctx, stack.SQLDB, secondAdminID, adminRoleID)
	var guardArmed bool
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT armed FROM admin_guard WHERE singleton = TRUE`).Scan(&guardArmed))
	require.True(t, guardArmed)

	firstTx, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	secondTx, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = firstTx.Rollback()
		_ = secondTx.Rollback()
	})
	_, err = firstTx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`, firstAdminID, adminRoleID)
	require.NoError(t, err)
	_, err = secondTx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`, secondAdminID, adminRoleID)
	require.NoError(t, err)

	results := make(chan error, 2)
	var commits sync.WaitGroup
	commits.Add(2)
	go func() {
		defer commits.Done()
		results <- firstTx.Commit()
	}()
	go func() {
		defer commits.Done()
		results <- secondTx.Commit()
	}()
	commits.Wait()
	close(results)

	successes := 0
	failures := 0
	for commitErr := range results {
		if commitErr == nil {
			successes++
		} else {
			failures++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, failures)

	remainingAdminID := requireSingleActiveAdmin(t, ctx, stack.SQLDB, adminRoleID)
	_, err = stack.SQLDB.ExecContext(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, remainingAdminID)
	require.Error(t, err, "last active administrator must not be suspended")
	_, err = stack.SQLDB.ExecContext(ctx, `UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = $1`, remainingAdminID)
	require.Error(t, err, "last active administrator must not be soft deleted")
	_, err = stack.SQLDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, remainingAdminID)
	require.Error(t, err, "last active administrator must not be hard deleted")

	replacementAdminID := insertActiveGuardUser(t, ctx, stack.SQLDB)
	transfer, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = transfer.ExecContext(ctx, `
		INSERT INTO user_roles (user_id, role_id, granted_by)
		VALUES ($1, $2, $1)
	`, replacementAdminID, adminRoleID)
	require.NoError(t, err)
	_, err = transfer.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`, remainingAdminID, adminRoleID)
	require.NoError(t, err)
	require.NoError(t, transfer.Commit())
	require.Equal(t, replacementAdminID, requireSingleActiveAdmin(t, ctx, stack.SQLDB, adminRoleID))

	viewerRole := model.Role{Name: "guard-viewer", DisplayName: "Guard Viewer"}
	require.NoError(t, stack.DB.Create(&viewerRole).Error)
	_, err = stack.SQLDB.ExecContext(ctx, `UPDATE user_roles SET role_id = $1 WHERE user_id = $2 AND role_id = $3`, viewerRole.ID, replacementAdminID, adminRoleID)
	require.Error(t, err, "last active administrator assignment must not be changed")
	_, err = stack.SQLDB.ExecContext(ctx, `UPDATE roles SET name = 'former-admin' WHERE id = $1`, adminRoleID)
	require.Error(t, err, "administrator role must not be renamed while it is the last active role")
	_, err = stack.SQLDB.ExecContext(ctx, `UPDATE roles SET deleted_at = CURRENT_TIMESTAMP WHERE id = $1`, adminRoleID)
	require.Error(t, err, "administrator role must not be soft deleted while it is the last active role")
	_, err = stack.SQLDB.ExecContext(ctx, `DELETE FROM roles WHERE id = $1`, adminRoleID)
	require.Error(t, err, "administrator role must not be hard deleted while it is the last active role")
	_, err = stack.SQLDB.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, adminRoleID)
	require.Error(t, err, "administrator recovery permission must not be cleared")
	_, err = stack.SQLDB.ExecContext(ctx, `DELETE FROM casbin_rule WHERE v0 = $1 AND v2 = 'PUT'`, adminRoleID)
	require.Error(t, err, "administrator recovery policies must not be cleared")

	_, err = stack.SQLDB.ExecContext(ctx, `DELETE FROM admin_guard WHERE singleton = TRUE`)
	require.Error(t, err, "administrator guard singleton must not be removable")
}

func TestActiveAdministratorGuardSerializesConcurrentSuspensions(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminRoleID := ensureGuardAdminRole(t, stack)
	firstAdminID := insertActiveGuardUser(t, ctx, stack.SQLDB)
	secondAdminID := insertActiveGuardUser(t, ctx, stack.SQLDB)
	insertGuardAdminAssignment(t, ctx, stack.SQLDB, firstAdminID, adminRoleID)
	insertGuardAdminAssignment(t, ctx, stack.SQLDB, secondAdminID, adminRoleID)

	firstTx, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	secondTx, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = firstTx.Rollback()
		_ = secondTx.Rollback()
	})
	_, err = firstTx.ExecContext(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, firstAdminID)
	require.NoError(t, err)
	_, err = secondTx.ExecContext(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, secondAdminID)
	require.NoError(t, err)

	results := collectCommitResults(startConcurrentCommits(firstTx, secondTx))
	require.Equal(t, 1, countNilErrors(results))
	require.Equal(t, []string{"23514"}, failedSQLStates(t, results))
	require.NotZero(t, requireSingleActiveAdmin(t, ctx, stack.SQLDB, adminRoleID))
}

func TestAdministratorGuardAllowsBootstrapBeforeItArms(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx := context.Background()
	userID := insertActiveGuardUser(t, ctx, stack.SQLDB)
	_, err := stack.SQLDB.ExecContext(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, userID)
	require.NoError(t, err)

	var guardArmed bool
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT armed FROM admin_guard WHERE singleton = TRUE`).Scan(&guardArmed))
	require.False(t, guardArmed)

	adminRoleID := ensureGuardAdminRole(t, stack)
	adminID := insertActiveGuardUser(t, ctx, stack.SQLDB)
	insertGuardAdminAssignment(t, ctx, stack.SQLDB, adminID, adminRoleID)
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT armed FROM admin_guard WHERE singleton = TRUE`).Scan(&guardArmed))
	require.True(t, guardArmed)
}

func ensureGuardAdminRole(t *testing.T, stack *integrationStack) uint64 {
	t.Helper()
	role := model.Role{Name: model.RoleAdmin, DisplayName: "Admin", IsSystem: true}
	require.NoError(t, stack.DB.Where("name = ?", model.RoleAdmin).FirstOrCreate(&role).Error)
	if !role.IsSystem {
		require.NoError(t, stack.DB.Model(&role).Update("is_system", true).Error)
	}
	ensureAdminRecoveryAuthority(t, stack, role.ID)
	return role.ID
}

func insertActiveGuardUser(t *testing.T, ctx context.Context, db *sql.DB) uint64 {
	t.Helper()
	var userID uint64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO users (primary_login_type, status)
		VALUES ('email', 'active')
		RETURNING id
	`).Scan(&userID))
	return userID
}

func insertGuardAdminAssignment(t *testing.T, ctx context.Context, db *sql.DB, userID, roleID uint64) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_roles (user_id, role_id, granted_by)
		VALUES ($1, $2, $1)
	`, userID, roleID)
	require.NoError(t, err)
}

func requireSingleActiveAdmin(t *testing.T, ctx context.Context, db *sql.DB, roleID uint64) uint64 {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT users.id
		FROM users
		JOIN user_roles ON user_roles.user_id = users.id
		WHERE user_roles.role_id = $1
		  AND users.status = 'active'
		  AND users.deleted_at IS NULL
	`, roleID)
	require.NoError(t, err)
	defer rows.Close()

	adminIDs := make([]uint64, 0, 1)
	for rows.Next() {
		var adminID uint64
		require.NoError(t, rows.Scan(&adminID))
		adminIDs = append(adminIDs, adminID)
	}
	require.NoError(t, rows.Err())
	require.Len(t, adminIDs, 1)
	return adminIDs[0]
}
