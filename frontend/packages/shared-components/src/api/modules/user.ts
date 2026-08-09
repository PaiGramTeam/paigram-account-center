import type { createRequest } from '../request'
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
} from '../types'

export function createUserApi(request: ReturnType<typeof createRequest>) {
  return {
    // List users with pagination and search filters.
    async getList(params?: UserListParams): Promise<UserListResponse> {
      const response = await request.get<UserListResponse>('/users', { params })
      return response.data
    },

    // Get one user.
    async getDetail(id: number | string): Promise<{ data: UserDetail }> {
      return request.get(`/users/${id}`)
    },

    // Create a user as an administrator.
    async create(data: CreateUserRequest): Promise<CreateUserResponse> {
      return request.post('/users', data)
    },

    // Update a user as an administrator.
    async update(id: number | string, data: UpdateUserRequest): Promise<UpdateUserResponse> {
      return request.patch(`/users/${id}`, data)
    },

    // Delete a user as an administrator.
    async delete(id: number | string, hardDelete = false): Promise<{ data: { message: string } }> {
      return request.delete(`/users/${id}`, { params: { hard_delete: hardDelete } })
    },

    // Update a user's status.
    async updateStatus(id: number | string, status: string): Promise<{ data: { id: number; status: string } }> {
      return request.patch(`/users/${id}/status`, { status })
    },

    // Reset a user's password.
    async resetPassword(id: number | string): Promise<{ data: { message: string } }> {
      return request.post(`/users/${id}/reset-password`)
    },

    // List a user's audit logs.
    async getAuditLogs(id: number | string, params?: { page?: number; page_size?: number; action_type?: string }) {
      return request.get<PaginatedResponse<UserAuditLogItem>>(`/users/${id}/audit-logs`, { params })
    },

    // List a user's login logs.
    async getLoginLogs(
      id: number | string,
      params?: { page?: number; page_size?: number; status?: 'success' | 'failed' }
    ) {
      return request.get<PaginatedResponse<UserLoginLogItem>>(`/users/${id}/login-logs`, { params })
    },

    // List a user's sessions.
    async getSessions(id: number | string, params?: { page?: number; page_size?: number }) {
      return request.get<PaginatedResponse<UserSessionItem>>(`/users/${id}/sessions`, { params })
    },

    // Revoke a user session.
    async revokeSession(id: number | string, sessionId: number | string): Promise<{ data: { message: string } }> {
      return request.delete(`/users/${id}/sessions/${sessionId}`)
    },

    // Get a user's security summary.
    async getSecuritySummary(id: number | string): Promise<{ data: UserSecuritySummary }> {
      return request.get(`/users/${id}/security-summary`)
    },
  }
}

export function createProfileApi(request: ReturnType<typeof createRequest>) {
  return {
    // Get a user profile.
    async getProfile(id: number | string): Promise<ProfileResponse> {
      return request.get(`/profiles/${id}`)
    },

    // Update a user profile.
    async updateProfile(id: number | string, data: UpdateProfileRequest): Promise<{ data: ProfileData }> {
      return request.patch(`/profiles/${id}`, data)
    },

    // List accounts bound to a profile.
    async getBoundAccounts(id: number | string): Promise<PaginatedResponse<BoundAccount>> {
      return request.get(`/profiles/${id}/accounts`)
    },

    // Bind an external account to a profile.
    async bindAccount(id: number | string, data: BindAccountRequest): Promise<BindAccountResponse> {
      return request.post(`/profiles/${id}/accounts/bind`, data)
    },

    // Unbind an external account from a profile.
    async unbindAccount(id: number | string, provider: string): Promise<UnbindAccountResponse> {
      return request.delete(`/profiles/${id}/accounts/${provider}`)
    },
  }
}
