import type { createRequest } from '../request'
import type { components } from '../generated/schema'
import type {
  ProfileData,
  ProfileResponse,
  UpdateProfileRequest,
  BoundAccount,
  PaginatedResponse,
  BindAccountRequest,
  BindAccountResponse,
  UnbindAccountResponse,
  UserListParams,
  UserListResponse,
  CreateUserRequest,
  CreateUserResponse,
  UpdateUserRequest,
  UpdateUserResponse,
  UserDetail,
  UserAuditLogItem,
  UserLoginLogItem,
  UserSecuritySummary,
  UserSessionItem,
  UserStatus,
  LoginType,
  ActivityLogItem,
  ResetPasswordRequest,
} from '../types'

type BackendUserDetail = components['schemas']['UserDetail']
type BackendUserListItem = components['schemas']['UserListItem']

const userStatuses: UserStatus[] = ['active', 'pending', 'suspended', 'deleted']
const loginTypes: LoginType[] = ['email', 'telegram', 'google', 'github']

function normalizeUserStatus(status: string): UserStatus {
  return userStatuses.includes(status as UserStatus) ? (status as UserStatus) : 'pending'
}

function normalizeLoginType(loginType: string): LoginType {
  return loginTypes.includes(loginType as LoginType) ? (loginType as LoginType) : 'email'
}

function mapUserListItem(user: BackendUserListItem) {
  return {
    ...user,
    status: normalizeUserStatus(user.status),
    primary_login_type: normalizeLoginType(user.primary_login_type),
    roles: user.roles ?? undefined,
  }
}

function mapUserDetail(user: BackendUserDetail): UserDetail {
  return {
    ...user,
    primary_email: user.primary_email ?? '',
    status: normalizeUserStatus(user.status),
    primary_login_type: normalizeLoginType(user.primary_login_type),
    emails: user.emails ?? undefined,
    permissions: user.permissions ?? undefined,
    roles: user.roles ?? undefined,
  }
}

export function createUserApi(request: ReturnType<typeof createRequest>) {
  return {
    // List users with pagination and search filters.
    async getList(params?: UserListParams): Promise<UserListResponse> {
      const response = await request.get<components['schemas']['UserListResponseDataStruct']>('/admin/users', {
        params,
      })
      return {
        data: (response.data.items ?? []).map(mapUserListItem),
        pagination: response.data.pagination,
      }
    },

    // Get one user.
    async getDetail(id: number | string): Promise<{ data: UserDetail }> {
      const response = await request.get<BackendUserDetail>(`/admin/users/${id}`)
      return { data: mapUserDetail(response.data) }
    },

    // Create a user as an administrator.
    async create(data: CreateUserRequest): Promise<CreateUserResponse> {
      return request.post('/admin/users', { primary_login_type: 'email', ...data })
    },

    // Update a user as an administrator.
    async update(id: number | string, data: UpdateUserRequest): Promise<UpdateUserResponse> {
      return request.patch(`/admin/users/${id}`, data)
    },

    // Delete a user as an administrator.
    async delete(id: number | string, hardDelete = false): Promise<{ data: { message: string } }> {
      return request.delete(`/admin/users/${id}`, { params: { hard_delete: hardDelete } })
    },

    // Update a user's status.
    async updateStatus(id: number | string, status: string): Promise<{ data: { id: number; status: string } }> {
      return request.patch(`/admin/users/${id}/status`, { status })
    },

    // Reset a user's password.
    async resetPassword(id: number | string, data: ResetPasswordRequest): Promise<{ data: { message: string } }> {
      return request.post(`/admin/users/${id}/reset-password`, data)
    },

    // List a user's audit logs.
    async getAuditLogs(id: number | string, params?: { page?: number; page_size?: number; action_type?: string }) {
      const response = await request.get<components['schemas']['AuditLogsResponseDataStruct']>(
        `/admin/users/${id}/audit-logs`,
        { params }
      )
      return {
        data: {
          data: {
            data: (response.data.items ?? []).map((item) => ({
              ...item,
              details: typeof item.details === 'string' ? item.details : JSON.stringify(item.details),
            })) satisfies UserAuditLogItem[],
            pagination: response.data.pagination,
          },
        },
      }
    },

    // List a user's login logs.
    async getLoginLogs(
      id: number | string,
      params?: { page?: number; page_size?: number; status?: 'success' | 'failed' }
    ) {
      const response = await request.get<components['schemas']['LoginLogsResponseDataStruct']>(
        `/admin/users/${id}/login-logs`,
        { params }
      )
      return {
        data: {
          data: {
            data: (response.data.items ?? []).map((item) => ({
              ...item,
              status: item.status === 'success' ? ('success' as const) : ('failed' as const),
            })) satisfies UserLoginLogItem[],
            pagination: response.data.pagination,
          },
        },
      }
    },

    // List a user's sessions.
    async getSessions(id: number | string, params?: { page?: number; page_size?: number }) {
      const response = await request.get<components['schemas']['PaginatedDataSessionResponse']>(
        `/admin/users/${id}/sessions`,
        { params }
      )
      return {
        data: {
          data: {
            data: (response.data.items ?? []) satisfies UserSessionItem[],
            pagination: response.data.pagination,
          },
        },
      }
    },

    // Revoke a user session.
    async revokeSession(id: number | string, sessionId: number | string): Promise<{ data: { message: string } }> {
      return request.delete(`/admin/users/${id}/sessions/${sessionId}`)
    },

    // Get a user's security summary.
    async getSecuritySummary(id: number | string): Promise<{ data: UserSecuritySummary }> {
      return request.get(`/admin/users/${id}/security-summary`)
    },
  }
}

export function createProfileApi(request: ReturnType<typeof createRequest>) {
  return {
    // Get a user profile.
    async getProfile(_id: number | string): Promise<ProfileResponse> {
      const response = await request.get<components['schemas']['CurrentUserView']>('/me')
      return {
        data: {
          ...response.data,
          user_id: response.data.id,
          primary_email: response.data.primary_email ?? '',
          emails: response.data.emails ?? [],
        },
      }
    },

    // Update a user profile.
    async updateProfile(_id: number | string, data: UpdateProfileRequest): Promise<{ data: ProfileData }> {
      const response = await request.patch<components['schemas']['CurrentUserView']>('/me', data)
      return {
        data: {
          ...response.data,
          user_id: response.data.id,
          primary_email: response.data.primary_email ?? '',
          emails: response.data.emails ?? [],
        },
      }
    },

    // List accounts bound to a profile.
    async getBoundAccounts(_id: number | string): Promise<PaginatedResponse<BoundAccount>> {
      const response = await request.get<components['schemas']['LoginMethodView'][] | null>('/me/login-methods')
      const accounts = (response.data ?? []).map((method) => ({
        provider: method.provider,
        provider_account_id: method.provider_account_id,
        display_name: method.display_name,
        avatar_url: method.avatar_url,
        bound_at: method.created_at,
        last_used_at: method.updated_at,
        is_primary: method.is_primary,
      }))
      return {
        data: {
          data: accounts,
          pagination: { page: 1, page_size: accounts.length, total: accounts.length, total_pages: 1 },
        },
      }
    },

    // Bind an external account to a profile.
    async bindAccount(_id: number | string, data: BindAccountRequest): Promise<BindAccountResponse> {
      return request.put(`/me/login-methods/${data.provider}`, { redirect_to: data.redirect_to })
    },

    // Unbind an external account from a profile.
    async unbindAccount(_id: number | string, provider: string): Promise<UnbindAccountResponse> {
      return request.delete(`/me/login-methods/${provider}`)
    },

    async getActivityLogs(params?: { page?: number; page_size?: number; action_type?: string }) {
      const response = await request.get<components['schemas']['PaginatedDataActivityLogView']>('/me/activity-logs', {
        params,
      })
      return {
        data: {
          data: {
            data: (response.data.items ?? []) satisfies ActivityLogItem[],
            pagination: response.data.pagination,
          },
        },
      }
    },
  }
}
