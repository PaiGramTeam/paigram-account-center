package casbin

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	internalcasbin "paigram/internal/casbin"
)

func TestMigratePermissionsToCasbinPreservesCustomPolicies(t *testing.T) {
	internalcasbin.Reset()
	t.Cleanup(internalcasbin.Reset)

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, createTestRolesTable(db))
	require.NoError(t, db.Exec("CREATE TABLE IF NOT EXISTS permissions (id INTEGER PRIMARY KEY, name TEXT NOT NULL)").Error)
	require.NoError(t, db.Exec("CREATE TABLE IF NOT EXISTS role_permissions (role_id INTEGER NOT NULL, permission_id INTEGER NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO roles (id, name, display_name) VALUES (?, ?, ?)", 11, "role-11", "Role 11").Error)
	require.NoError(t, db.Exec("INSERT INTO permissions (id, name) VALUES (?, ?)", 101, "role:manage").Error)
	require.NoError(t, db.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", 11, 101).Error)

	_, err = internalcasbin.InitEnforcer(db)
	require.NoError(t, err)

	enforcer := internalcasbin.GetEnforcer()
	_, err = enforcer.AddPolicy("11", "/api/v1/custom/authority-policy", "GET")
	require.NoError(t, err)
	require.NoError(t, enforcer.LoadPolicy())

	service := &CasbinService{db: db}
	require.NoError(t, service.MigratePermissionsToCasbin())

	policies := enforcer.GetFilteredPolicy(0, fmt.Sprint(11))
	assert.Contains(t, policies, []string{"11", "/api/v1/custom/authority-policy", "GET"})
	assert.Contains(t, policies, []string{"11", "/api/v1/admin/roles/:id/users", "GET"})
	assert.Contains(t, policies, []string{"11", "/api/v1/admin/roles/:id/users", "PUT"})
	assert.Contains(t, policies, []string{"11", "/api/v1/admin/roles/:id/permissions", "GET"})
	assert.Contains(t, policies, []string{"11", "/api/v1/admin/roles/:id/permissions", "PUT"})
	assert.Contains(t, policies, []string{"11", "/api/v1/admin/users/:id/roles", "PUT"})
	assert.Contains(t, policies, []string{"11", "/api/v1/admin/users/:id/primary-role", "PATCH"})
	assert.NotContains(t, policies, []string{"11", "/api/v1/casbin/authorities/:id/policies", "GET"})
	assert.NotContains(t, policies, []string{"11", "/api/v1/casbin/authorities/:id/policies", "PUT"})
	assert.NotContains(t, policies, []string{"11", "/api/v1/authorities/:id/users", "GET"})
}

func TestPoliciesForPermissionNameRejectsLegacyPermissionVocabulary(t *testing.T) {
	for _, permissionName := range []string{"user:write", "user:manage", "role:write", "permission:write", "bot:write"} {
		assert.Empty(t, policiesForPermissionName(permissionName), permissionName)
	}
}
