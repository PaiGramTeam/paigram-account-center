import type { components, operations } from './generated/schema'

export type OpenApiCurrentUser = components['schemas']['CurrentUserView']
export type OpenApiLoginMethod = components['schemas']['LoginMethodView']
export type OpenApiPermission = components['schemas']['Permission']
export type OpenApiPermissionInfo = components['schemas']['PermissionInfo']
export type OpenApiRole = components['schemas']['RoleWithPermissions']
export type OpenApiUserDetail = components['schemas']['UserDetail']
export type OpenApiUserListItem = components['schemas']['UserListItem']

export type OpenApiLoginRequest = operations['post-api-v1-auth-login']['requestBody']['content']['application/json']
export type OpenApiLoginResponse = operations['post-api-v1-auth-login']['responses'][200]['content']['application/json']
export type OpenApiMeResponse = operations['get-api-v1-me']['responses'][200]['content']['application/json']
export type OpenApiAdminUserListResponse =
  operations['get-api-v1-admin-users']['responses'][200]['content']['application/json']
export type OpenApiRoleListResponse =
  operations['get-api-v1-admin-roles']['responses'][200]['content']['application/json']

export type { components, operations, paths } from './generated/schema'
