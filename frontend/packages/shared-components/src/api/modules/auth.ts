import type { createRequest } from '../request'
import type {
  LoginEmailRequest,
  LoginEmailResponse,
  LoginResponse,
  RefreshTokenRequest,
  LogoutRequest,
  LogoutResponse,
  RegisterEmailRequest,
  RegisterEmailResponse,
  InitiateOAuthRequest,
  InitiateOAuthResponse,
  OAuthCallbackRequest,
  TelegramAuthData,
  TelegramAuthResponse,
} from '../types'

export function createAuthApi(request: ReturnType<typeof createRequest>) {
  return {
    // 邮箱密码登录
    async login(data: LoginEmailRequest): Promise<LoginEmailResponse> {
      return request.post('/auth/login', data, { skipErrorToast: true })
    },

    // 刷新 Token
    async refreshToken(data: RefreshTokenRequest): Promise<LoginResponse> {
      return request.post('/auth/refresh', data)
    },

    // 登出
    async logout(data: LogoutRequest): Promise<LogoutResponse> {
      return request.post('/auth/logout', data)
    },

    // 邮箱注册
    async register(data: RegisterEmailRequest): Promise<RegisterEmailResponse> {
      return request.post('/auth/register', data, { skipErrorToast: true })
    },

    // 邮箱验证
    async verifyEmail(data: { email: string; token: string }): Promise<{ data: { message: string } }> {
      return request.post('/auth/verify-email', data)
    },

    // 初始化 OAuth 登录
    async initiateOAuth(provider: string, data?: InitiateOAuthRequest): Promise<InitiateOAuthResponse> {
      return request.post(`/auth/oauth/${provider}/init`, data, { skipErrorToast: true })
    },

    // OAuth 回调处理
    async handleOAuthCallback(provider: string, data: OAuthCallbackRequest): Promise<LoginResponse> {
      return request.post(`/auth/oauth/${provider}/callback`, data, { skipErrorToast: true })
    },

    // Telegram 登录
    async telegramAuth(data: TelegramAuthData, botToken: string): Promise<TelegramAuthResponse> {
      const response = await request.post('/auth/oauth/telegram', data, {
        skipErrorToast: true,
        headers: {
          'X-Telegram-Bot-Token': botToken,
        },
      })
      return response.data as TelegramAuthResponse
    },
  }
}
