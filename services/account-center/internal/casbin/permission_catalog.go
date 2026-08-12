package casbin

import "paigram/internal/model"

// PermissionDefinition is the authoritative metadata for one assignable permission.
type PermissionDefinition struct {
	Name        string
	Resource    string
	Action      string
	Description string
}

var permissionDefinitions = []PermissionDefinition{
	{Name: model.PermUserCreate, Resource: model.ResourceUser, Action: model.ActionCreate, Description: "Create new users"},
	{Name: model.PermUserRead, Resource: model.ResourceUser, Action: model.ActionRead, Description: "View user information"},
	{Name: model.PermUserUpdate, Resource: model.ResourceUser, Action: model.ActionUpdate, Description: "Update user information"},
	{Name: model.PermUserDelete, Resource: model.ResourceUser, Action: model.ActionDelete, Description: "Delete users"},
	{Name: model.PermUserList, Resource: model.ResourceUser, Action: model.ActionList, Description: "List all users"},

	{Name: model.PermRoleCreate, Resource: model.ResourceRole, Action: model.ActionCreate, Description: "Create new roles"},
	{Name: model.PermRoleRead, Resource: model.ResourceRole, Action: model.ActionRead, Description: "View role information"},
	{Name: model.PermRoleUpdate, Resource: model.ResourceRole, Action: model.ActionUpdate, Description: "Update role information"},
	{Name: model.PermRoleDelete, Resource: model.ResourceRole, Action: model.ActionDelete, Description: "Delete roles"},
	{Name: model.PermRoleList, Resource: model.ResourceRole, Action: model.ActionList, Description: "List all roles"},
	{Name: model.PermRoleManage, Resource: model.ResourceRole, Action: model.ActionManage, Description: "Manage role assignments"},

	{Name: model.PermPermissionCreate, Resource: model.ResourcePermission, Action: model.ActionCreate, Description: "Create new permissions"},
	{Name: model.PermPermissionRead, Resource: model.ResourcePermission, Action: model.ActionRead, Description: "View permission information"},
	{Name: model.PermPermissionDelete, Resource: model.ResourcePermission, Action: model.ActionDelete, Description: "Delete permissions"},
	{Name: model.PermPermissionList, Resource: model.ResourcePermission, Action: model.ActionList, Description: "List all permissions"},

	{Name: model.PermSystemRead, Resource: model.ResourceSystem, Action: model.ActionRead, Description: "View system settings and auth controls"},
	{Name: model.PermSystemUpdate, Resource: model.ResourceSystem, Action: model.ActionUpdate, Description: "Update system settings and auth controls"},

	{Name: model.PermPlatformCreate, Resource: model.ResourcePlatform, Action: model.ActionCreate, Description: "Create platform registrations"},
	{Name: model.PermPlatformRead, Resource: model.ResourcePlatform, Action: model.ActionRead, Description: "View platform registration information"},
	{Name: model.PermPlatformUpdate, Resource: model.ResourcePlatform, Action: model.ActionUpdate, Description: "Update platform registrations"},
	{Name: model.PermPlatformDelete, Resource: model.ResourcePlatform, Action: model.ActionDelete, Description: "Delete platform registrations"},
	{Name: model.PermPlatformList, Resource: model.ResourcePlatform, Action: model.ActionList, Description: "List platform registrations"},
	{Name: model.PermPlatformManage, Resource: model.ResourcePlatform, Action: model.ActionManage, Description: "Manage platform registrations"},

	{Name: model.PermPlatformAccountRead, Resource: model.ResourcePlatformAccount, Action: model.ActionRead, Description: "View platform account bindings"},
	{Name: model.PermPlatformAccountList, Resource: model.ResourcePlatformAccount, Action: model.ActionList, Description: "List platform account bindings"},
	{Name: model.PermPlatformAccountUpdate, Resource: model.ResourcePlatformAccount, Action: model.ActionUpdate, Description: "Update platform account bindings"},
	{Name: model.PermPlatformAccountDelete, Resource: model.ResourcePlatformAccount, Action: model.ActionDelete, Description: "Delete platform account bindings"},

	{Name: model.PermBotCreate, Resource: model.ResourceBot, Action: model.ActionCreate, Description: "Create new bots"},
	{Name: model.PermBotRead, Resource: model.ResourceBot, Action: model.ActionRead, Description: "View bot information"},
	{Name: model.PermBotUpdate, Resource: model.ResourceBot, Action: model.ActionUpdate, Description: "Update bot information"},
	{Name: model.PermBotDelete, Resource: model.ResourceBot, Action: model.ActionDelete, Description: "Delete bots"},
	{Name: model.PermBotList, Resource: model.ResourceBot, Action: model.ActionList, Description: "List all bots"},
	{Name: model.PermBotManage, Resource: model.ResourceBot, Action: model.ActionManage, Description: "Manage bot tokens"},

	{Name: model.PermSessionRead, Resource: model.ResourceSession, Action: model.ActionRead, Description: "View session information"},
	{Name: model.PermSessionDelete, Resource: model.ResourceSession, Action: model.ActionDelete, Description: "Revoke sessions"},
	{Name: model.PermSessionList, Resource: model.ResourceSession, Action: model.ActionList, Description: "List all sessions"},

	{Name: model.PermAuditRead, Resource: model.ResourceAudit, Action: model.ActionRead, Description: "View audit logs"},
	{Name: model.PermAuditList, Resource: model.ResourceAudit, Action: model.ActionList, Description: "List audit logs"},
}

var retiredPermissionNames = []string{
	"user:write",
	"user:manage",
	"role:write",
	"permission:write",
	"permission:manage",
	"bot:write",
}

// AllPermissionDefinitions returns a copy of the assignable permission catalog.
func AllPermissionDefinitions() []PermissionDefinition {
	return append([]PermissionDefinition(nil), permissionDefinitions...)
}

// RetiredPermissionNames returns permission names that must not remain assignable.
func RetiredPermissionNames() []string {
	return append([]string(nil), retiredPermissionNames...)
}
