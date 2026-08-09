export interface ApiResponse<T = unknown> {
  code?: number
  message?: string
  data: T
  timestamp?: number
}

export interface ApiErrorDetail {
  code?: string
  message?: string
  details?: Record<string, unknown>
}

export interface ApiError {
  error?: string | ApiErrorDetail
  code?: string
  message?: string
  details?: Record<string, unknown>
}

export interface LoginEmailRequest {
  email: string
  password: string
  totp_code?: string
  trust_device?: boolean
  captcha_token?: string
}

export interface LoginChallengeResponseData {
  requires_totp: true
  message: string
}

export interface LoginResponseData {
  access_token: string
  refresh_token: string
  access_expiry: string
  refresh_expiry: string
  user_id: number
}

export type LoginEmailResponseData = LoginResponseData | LoginChallengeResponseData
export type LoginResponse = ApiResponse<LoginResponseData>
export type LoginChallengeResponse = ApiResponse<LoginChallengeResponseData>
export type LoginEmailResponse = ApiResponse<LoginEmailResponseData>

export interface UserInfo {
  id: number
  display_name: string
  avatar_url?: string
  primary_email: string
  status: UserStatus
  created_at: string
  updated_at: string
  last_login_at?: string
  bio?: string
  locale?: string
  roles?: string[]
  permissions?: string[]
}

export interface UserDetail extends UserInfo {
  username?: string
  nickname?: string
  email?: string
  email_verified?: boolean
  phone?: string
  phone_verified?: boolean
  permissions?: string[]
  last_login_ip?: string
  last_login_device?: string
  login_methods?: LoginType[]
  two_factor_enabled?: boolean
  active_session_count?: number
  emails?: EmailData[]
  primary_login_type?: LoginType
}

export interface UserListItem {
  id: number
  display_name: string
  primary_email: string
  avatar_url?: string
  status: UserStatus
  roles?: string[]
  primary_login_type?: LoginType
  last_login_at?: string
  created_at: string
}

export type UserStatus = 'active' | 'pending' | 'suspended' | 'deleted'
export type LoginType = 'email' | 'telegram' | 'google' | 'github'

export interface PaginationParams {
  page?: number
  page_size?: number
  sort_by?: string
  order?: 'asc' | 'desc'
}

export interface UserListParams extends PaginationParams {
  status?: UserStatus
  search?: string
}

export interface PaginationMeta {
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface UserListResponse {
  data: UserListItem[]
  pagination: PaginationMeta
}

export interface CreateUserRequest {
  email: string
  display_name: string
  password: string
  locale?: string
  roles?: string[]
  status?: UserStatus
}

export interface CreateUserResponse {
  data: UserDetail
}

export interface UpdateUserRequest {
  display_name?: string
  status?: UserStatus
  roles?: string[]
  locale?: string
}

export interface UpdateUserResponse {
  data: UserDetail
}

export interface UserSessionItem {
  id: number
  device_id?: string
  device_name?: string
  device_type?: string
  ip?: string
  location?: string
  created_at: string
  last_active_at?: string
  access_expiry: string
  refresh_expiry: string
  is_current: boolean
}

export interface UserAuditLogItem {
  id: number
  user_id: number
  action: string
  details?: string
  ip?: string
  created_at: string
}

export interface UserSecuritySummary {
  user_id: number
  two_factor_enabled: boolean
  active_session_count: number
  device_count: number
  failed_logins_last_30_days: number
  last_login_at?: string
  last_login_ip?: string
  last_login_device?: string
  last_login_location?: string
}

export interface UserLoginLogItem {
  id: number
  user_id: number
  login_type: string
  ip: string
  user_agent?: string
  device?: string
  location?: string
  status: 'success' | 'failed'
  failure_reason?: string
  created_at: string
}

export interface RefreshTokenRequest {
  refresh_token: string
}

export interface LogoutRequest {
  token: string
}

export interface LogoutResponse {
  data: {
    message: string
  }
}

export interface RegisterEmailRequest {
  email: string
  password: string
  display_name: string
  locale?: string
  captcha_token?: string
}

export interface RegisterEmailResponse {
  data: {
    user_id: number
    email: string
    requires_email_verification: boolean
    verification_token?: string
    verification_expires_at?: string
  }
}

export interface ProfileData {
  user_id: number
  display_name: string
  avatar_url?: string
  bio?: string
  primary_email: string
  emails: EmailData[]
  status: string
  locale?: string
  created_at: string
  updated_at: string
  last_login_at?: string
}

export interface EmailData {
  email: string
  is_primary: boolean
  verified_at?: string
}

export interface ProfileResponse {
  data: ProfileData
}

export interface UpdateProfileRequest {
  display_name?: string
  avatar_url?: string
  bio?: string
  locale?: string
}

export interface InitiateOAuthRequest {
  redirect_to?: string
}

export interface InitiateOAuthResponse {
  data: {
    auth_url: string
    state: string
    nonce: string
    expires_at: string
  }
}

export interface OAuthCallbackRequest {
  state: string
  code: string
}

// Telegram Auth
export interface TelegramAuthData {
  id: number
  first_name?: string
  last_name?: string
  username?: string
  photo_url?: string
  auth_date: number
  hash: string
}

export interface TelegramAuthResponse {
  access_token: string
  refresh_token: string
  expires_in: number
  token_type: string
  user: {
    id: number
    display_name: string
    email?: string
    avatar_url?: string
    status: string
  }
}

export interface BoundAccount {
  provider: string
  provider_account_id: string
  display_name?: string
  avatar_url?: string
  email?: string
  bound_at: string
  last_used_at?: string
  is_primary?: boolean
}

export interface PaginatedResponse<T> {
  code?: number
  message?: string
  data: {
    data: T[]
    pagination: {
      page: number
      page_size: number
      total: number
      total_pages: number
    }
  }
}

export interface BindAccountRequest {
  provider: string
  provider_data: Record<string, unknown>
}

export interface BindAccountResponse {
  data: {
    provider: string
    provider_account_id: string
    bound_at: string
  }
}

export interface UnbindAccountResponse {
  data: {
    message: string
  }
}

export interface ChangePasswordRequest {
  old_password: string
  new_password: string
}

export interface ChangePasswordResponse {
  data: {
    message: string
  }
}

export interface Enable2FAResponse {
  data: {
    qr_code: string
    secret: string
    backup_codes: string[]
  }
}

export interface Confirm2FARequest {
  code: string
  secret: string
}

export interface Confirm2FAResponse {
  data: {
    message: string
    backup_codes: string[]
  }
}

export interface Disable2FARequest {
  password: string
  code: string
}

export interface Disable2FAResponse {
  data: {
    message: string
  }
}

export interface Device {
  device_id: string
  device_name: string
  device_type?: string
  browser?: string
  os?: string
  ip: string
  location?: string
  last_active_at: string
  is_current: boolean
  trust_expiry?: string
}

export interface DevicesResponse {
  data: {
    data: Device[]
  }
}

export interface RemoveDeviceResponse {
  data: {
    message: string
  }
}

export interface RoleListItem {
  id: number
  name: string
  display_name: string
  description: string
  permission_count: number
  user_count?: number
  is_system?: boolean
  created_at: string
}

export interface RoleDetail {
  id: number
  name: string
  display_name: string
  description: string
  permission_count: number
  user_count?: number
  is_system?: boolean
  permissions?: PermissionListItem[]
  created_at: string
  updated_at: string
}

export interface RoleListParams extends PaginationParams {}

export interface RoleListResponse {
  data: RoleListItem[]
  pagination: PaginationMeta
}

export interface CreateRoleRequest {
  name: string
  display_name: string
  description?: string
}

export interface CreateRoleResponse {
  data: RoleDetail
}

export interface UpdateRoleRequest {
  display_name?: string
  description?: string
}

export interface UpdateRoleResponse {
  data: RoleDetail
}

export interface DeleteRoleResponse {
  data: {
    message: string
  }
}

export interface RoleDetailResponse {
  data: RoleDetail
}

export interface PermissionListItem {
  id: number
  name: string
  display_name: string
  description: string
  category: string
  resource: string
  action: string
  created_at?: string
  updated_at?: string
}

export interface PermissionDetail {
  id: number
  name: string
  display_name: string
  description: string
  category: string
  resource: string
  action: string
  created_at: string
  updated_at: string
}

export interface PermissionListParams extends PaginationParams {
  category?: string
}

export interface PermissionListResponse {
  data: PermissionListItem[]
  pagination: PaginationMeta
}

export interface PermissionDetailResponse {
  data: PermissionDetail
}

export interface CreatePermissionRequest {
  name: string
  display_name?: string
  description?: string
  category?: string
  resource?: string
  action?: string
}

export interface CreatePermissionResponse {
  data: PermissionDetail
}

export interface UpdatePermissionRequest {
  display_name?: string
  description?: string
  category?: string
  resource?: string
  action?: string
}

export interface UpdatePermissionResponse {
  data: PermissionDetail
}

export interface DeletePermissionResponse {
  data: {
    message: string
  }
}

export interface AssignPermissionRequest {
  permission_id: number
}

export interface AssignPermissionResponse {
  data: {
    message: string
  }
}
