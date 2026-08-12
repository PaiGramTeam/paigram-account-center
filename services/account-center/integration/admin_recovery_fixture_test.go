//go:build integration

package integration

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"paigram/internal/casbin"
	"paigram/internal/model"
)

func ensureAdminRecoveryAuthority(t *testing.T, stack *integrationStack, roleID uint64) {
	t.Helper()
	permission := model.Permission{
		Name:        model.PermRoleManage,
		Resource:    model.ResourceRole,
		Action:      model.ActionManage,
		Description: "Recover administrator membership and permissions",
	}
	require.NoError(t, stack.DB.Where("name = ?", permission.Name).FirstOrCreate(&permission).Error)
	rolePermission := model.RolePermission{RoleID: roleID, PermissionID: permission.ID}
	require.NoError(t, stack.DB.Where("role_id = ? AND permission_id = ?", roleID, permission.ID).FirstOrCreate(&rolePermission).Error)

	enforcer := casbin.GetEnforcer()
	require.NotNil(t, enforcer)
	for _, path := range []string{
		"/api/v1/admin/roles/:id/users",
		"/api/v1/admin/roles/:id/permissions",
	} {
		_, err := enforcer.AddPolicy(fmt.Sprint(roleID), path, "PUT")
		require.NoError(t, err)
	}
}
