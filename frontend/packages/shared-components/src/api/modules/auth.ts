import type { createRequest } from '../request'
import type {
  LoginEmailRequest,
  LoginEmailResponse,
  LoginResponse,
  LogoutResponse,
  RegisterEmailRequest,
  RegisterEmailResponse,
  InitiateOAuthRequest,
  InitiateOAuthResponse,
  OAuthCallbackRequest,
  OAuthCallbackResponse,
  ForgotPasswordRequest,
  PublicResetPasswordRequest,
} from '../types'

export function createAuthApi(request: ReturnType<typeof createRequest>) {
  return {
    async login(data: LoginEmailRequest): Promise<LoginEmailResponse> {
      return request.post('/auth/login', data, { skipErrorToast: true })
    },

    async refreshToken(): Promise<LoginResponse> {
      return request.post('/auth/refresh', undefined, { skipErrorToast: true })
    },

    async logout(): Promise<LogoutResponse> {
      return request.post('/auth/logout', undefined, { skipErrorToast: true })
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

    async handleOAuthCallback(provider: string, data: OAuthCallbackRequest): Promise<OAuthCallbackResponse> {
      return request.post(`/auth/oauth/${provider}/callback`, data, { skipErrorToast: true })
    },

    async forgotPassword(data: ForgotPasswordRequest): Promise<void> {
      await request.post('/auth/forgot-password', data, { skipErrorToast: true })
    },

    async resetPassword(data: PublicResetPasswordRequest): Promise<void> {
      await request.post('/auth/reset-password', data, { skipErrorToast: true })
    },
  }
}
