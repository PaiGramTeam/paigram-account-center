//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"paigram/internal/model"
	"paigram/internal/response"
)

func TestLastAdministratorMutationsReturnForbidden(t *testing.T) {
	stack := newIntegrationStack(t)
	adminID, accessToken, _, _, _ := registerAndLogin(
		t,
		stack,
		fmt.Sprintf("last-admin-%d@example.com", time.Now().UnixNano()),
		"AdminPass123!",
	)
	grantAdminRoleToUser(t, stack, adminID)
	var adminRole model.Role
	require.NoError(t, stack.DB.Where("name = ?", model.RoleAdmin).First(&adminRole).Error)

	tests := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{
			name:   "suspend user",
			method: http.MethodPatch,
			path:   fmt.Sprintf("/api/v1/admin/users/%d/status", adminID),
			body:   map[string]any{"status": model.UserStatusSuspended},
		},
		{
			name:   "remove role assignment",
			method: http.MethodPut,
			path:   fmt.Sprintf("/api/v1/admin/users/%d/roles", adminID),
			body:   map[string]any{"role_ids": []uint64{}},
		},
		{
			name:   "clear role members",
			method: http.MethodPut,
			path:   fmt.Sprintf("/api/v1/admin/roles/%d/users", adminRole.ID),
			body:   map[string]any{"user_ids": []uint64{}},
		},
		{
			name:   "clear recovery permissions",
			method: http.MethodPut,
			path:   fmt.Sprintf("/api/v1/admin/roles/%d/permissions", adminRole.ID),
			body:   map[string]any{"permission_ids": []uint64{}},
		},
		{
			name:   "hard delete user",
			method: http.MethodDelete,
			path:   fmt.Sprintf("/api/v1/admin/users/%d?hard_delete=true", adminID),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := performJSONRequest(t, stack.Router, test.method, test.path, test.body, authHeaders(accessToken))
			require.Equal(t, http.StatusForbidden, result.Code, result.Body.String())
			require.Equal(t, response.ErrCodePermissionDenied, decodeErrorCode(t, result))
		})
	}
}
