// API 响应基础类型
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

// API 错误响应
export interface ApiError {
  error?: string | ApiErrorDetail
  code?: string
  message?: string
  details?: Record<string, unknown>
}

// 登录相关类型
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

// 用户信息相关类型
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

// 扩展的用户详情类型（用于管理员面板）
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

// 用户列表项类型（用于用户管理列表）
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

// 分页参数
export interface PaginationParams {
  page?: number
  page_size?: number
  sort_by?: string
  order?: 'asc' | 'desc'
}

// 用户列表查询参数
export interface UserListParams extends PaginationParams {
  status?: UserStatus
  search?: string
}

// 分页元数据
export interface PaginationMeta {
  page: number
  page_size: number
  total: number
  total_pages: number
}

// 用户列表响应
export interface UserListResponse {
  data: UserListItem[]
  pagination: PaginationMeta
}

// 创建用户请求
export interface CreateUserRequest {
  email: string
  display_name: string
  password: string
  locale?: string
  roles?: string[]
  status?: UserStatus
}

// 创建用户响应
export interface CreateUserResponse {
  data: UserDetail
}

// 更新用户请求
export interface UpdateUserRequest {
  display_name?: string
  status?: UserStatus
  roles?: string[]
  locale?: string
}

// 更新用户响应
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

// 刷新 Token 请求
export interface RefreshTokenRequest {
  refresh_token: string
}

// 登出请求
export interface LogoutRequest {
  token: string
}

// 登出响应
export interface LogoutResponse {
  data: {
    message: string
  }
}

// 注册请求
export interface RegisterEmailRequest {
  email: string
  password: string
  display_name: string
  locale?: string
  captcha_token?: string
}

// 注册响应
export interface RegisterEmailResponse {
  data: {
    user_id: number
    email: string
    requires_email_verification: boolean
    verification_token?: string
    verification_expires_at?: string
  }
}

// Profile 相关类型
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

// OAuth 相关类型
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

// 账号绑定相关类型
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

// 安全设置相关类型

// 修改密码请求
export interface ChangePasswordRequest {
  old_password: string
  new_password: string
}

// 修改密码响应
export interface ChangePasswordResponse {
  data: {
    message: string
  }
}

// 启用 2FA 响应
export interface Enable2FAResponse {
  data: {
    qr_code: string // Base64 编码的 QR 码图片
    secret: string // TOTP 密钥
    backup_codes: string[] // 备用恢复码
  }
}

// 确认启用 2FA 请求
export interface Confirm2FARequest {
  code: string // TOTP 验证码
  secret: string // TOTP 密钥
}

// 确认启用 2FA 响应
export interface Confirm2FAResponse {
  data: {
    message: string
    backup_codes: string[]
  }
}

// 禁用 2FA 请求
export interface Disable2FARequest {
  password: string // 用户密码
  code: string // 当前的 TOTP 验证码
}

// 禁用 2FA 响应
export interface Disable2FAResponse {
  data: {
    message: string
  }
}

// 设备信息
export interface Device {
  device_id: string // 设备唯一标识符
  device_name: string // 设备显示名称，如 "Chrome 120.0 / Windows"
  device_type?: string // 设备类型，如 "desktop", "mobile"
  browser?: string // 浏览器名称
  os?: string // 操作系统
  ip: string // IP 地址
  location?: string // 地理位置，如 "Beijing, China"
  last_active_at: string // 最后活跃时间
  is_current: boolean // 是否是当前设备
  trust_expiry?: string // 信任过期时间（如果设备被信任）
}

// 获取设备列表响应
export interface DevicesResponse {
  data: {
    data: Device[]
  }
}

// 移除设备响应
export interface RemoveDeviceResponse {
  data: {
    message: string
  }
}

// 角色管理相关类型

// 角色列表项类型
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

// 角色详情类型
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

// 角色列表查询参数
export interface RoleListParams extends PaginationParams {
  // 可以根据需要添加其他筛选参数
}

// 角色列表响应
export interface RoleListResponse {
  data: RoleListItem[]
  pagination: PaginationMeta
}

// 创建角色请求
export interface CreateRoleRequest {
  name: string
  display_name: string
  description?: string
}

// 创建角色响应
export interface CreateRoleResponse {
  data: RoleDetail
}

// 更新角色请求
export interface UpdateRoleRequest {
  display_name?: string
  description?: string
}

// 更新角色响应
export interface UpdateRoleResponse {
  data: RoleDetail
}

// 删除角色响应
export interface DeleteRoleResponse {
  data: {
    message: string
  }
}

// 角色详情响应
export interface RoleDetailResponse {
  data: RoleDetail
}

// 权限管理相关类型

// 权限列表项类型
export interface PermissionListItem {
  id: number
  name: string // 权限唯一标识，如 "user:read"
  display_name: string // 显示名称，如 "User Read"
  description: string // 权限描述
  category: string // 权限分类，如 "user_management"
  resource: string
  action: string
  created_at?: string
  updated_at?: string
}

// 权限详情类型
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

// 权限列表查询参数
export interface PermissionListParams extends PaginationParams {
  category?: string // 按分类筛选
}

// 权限列表响应
export interface PermissionListResponse {
  data: PermissionListItem[]
  pagination: PaginationMeta
}

// 权限详情响应
export interface PermissionDetailResponse {
  data: PermissionDetail
}

// 创建权限请求
export interface CreatePermissionRequest {
  name: string
  display_name?: string
  description?: string
  category?: string
  resource?: string
  action?: string
}

// 创建权限响应
export interface CreatePermissionResponse {
  data: PermissionDetail
}

// 更新权限请求
export interface UpdatePermissionRequest {
  display_name?: string
  description?: string
  category?: string
  resource?: string
  action?: string
}

// 更新权限响应
export interface UpdatePermissionResponse {
  data: PermissionDetail
}

// 删除权限响应
export interface DeletePermissionResponse {
  data: {
    message: string
  }
}

// 分配权限给角色请求
export interface AssignPermissionRequest {
  permission_id: number
}

// 分配权限给角色响应
export interface AssignPermissionResponse {
  data: {
    message: string
  }
}
