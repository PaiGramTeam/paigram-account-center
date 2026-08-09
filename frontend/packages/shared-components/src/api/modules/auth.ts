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
} from '../types'

export function createAuthApi(request: ReturnType<typeof createRequest>) {
  return {
    async login(data: LoginEmailRequest): Promise<LoginEmailResponse> {
      return request.post('/auth/login', data, { skipErrorToast: true })
    },

    async refreshToken(data: RefreshTokenRequest): Promise<LoginResponse> {
      return request.post('/auth/refresh', data)
    },

    async logout(data: LogoutRequest): Promise<LogoutResponse> {
      return request.post('/auth/logout', data)
    },

    async register(data: RegisterEmailRequest): Promise<RegisterEmailResponse> {
      return request.post('/auth/register', data, { skipErrorToast: true })
    },

    async verifyEmail(data: { email: string; token: string }): Promise<{ data: { message: string } }> {
      return request.post('/auth/verify-email', data)
    },

    async initiateOAuth(provider: string, data?: InitiateOAuthRequest): Promise<InitiateOAuthResponse> {
      return request.post(`/auth/oauth/${provider}/init`, data, { skipErrorToast: true })
    },

    async handleOAuthCallback(provider: string, data: OAuthCallbackRequest): Promise<LoginResponse> {
      return request.post(`/auth/oauth/${provider}/callback`, data, { skipErrorToast: true })
    },
  }
}
