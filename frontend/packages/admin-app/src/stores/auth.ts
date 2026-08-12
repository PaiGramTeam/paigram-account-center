import { defineStore } from 'pinia'
import { Message } from '@arco-design/web-vue'
import { resolveAuthErrorMessage, useUserStore } from '@paigram/shared-components'
import { authApi, userApi } from '@/api'
import type {
  LoginChallengeResponseData,
  LoginEmailRequest,
  LoginResponseData,
  OAuthCallbackRequest,
  UserStatus,
} from '@paigram/shared-components'

interface AuthState {
  loading: boolean
  loginType: 'email' | 'oauth' | 'telegram' | null
}

export interface LoginWithEmailResult {
  status: 'success' | 'requires_totp'
  message?: string
}

export function resolveAdminPostLoginRoute(permissions: string[]): string {
  if (permissions.includes('platform_account:list')) {
    return '/platform-accounts'
  }

  if (permissions.includes('system:read')) {
    return '/system/settings'
  }

  if (permissions.includes('user:read')) {
    return '/dashboard'
  }

  if (permissions.includes('role:read')) {
    return '/users/roles'
  }

  if (permissions.includes('permission:read')) {
    return '/users/permissions'
  }

  if (permissions.includes('audit:list') || permissions.includes('audit:read')) {
    return '/system/settings'
  }

  return '/403'
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    loading: false,
    loginType: null,
  }),

  actions: {
    async loginWithEmail(credentials: LoginEmailRequest): Promise<LoginWithEmailResult> {
      this.loading = true
      const userStore = useUserStore()

      try {
        const response = await authApi.login(credentials)

        if (isTwoFactorChallenge(response.data)) {
          return {
            status: 'requires_totp',
            message: response.data.message,
          }
        }

        userStore.setAuthData({
          accessToken: response.data.access_token,
        })

        await this.fetchUserProfile(response.data.user_id)

        this.loginType = 'email'
        Message.success('登录成功')
        return { status: 'success' }
      } catch (error: unknown) {
        userStore.reset()
        Message.error(resolveAuthErrorMessage(error, '登录失败'))
        throw error
      } finally {
        this.loading = false
      }
    },

    async fetchUserProfile(userId?: number): Promise<void> {
      const userStore = useUserStore()

      try {
        const id = userId || userStore.userId
        if (!id) {
          throw new Error('User ID not found')
        }

        const response = await userApi.getDetail(id)
        const userDetail = response.data

        userStore.setUserInfo({
          id: userDetail.id,
          display_name: userDetail.display_name,
          primary_email: userDetail.primary_email,
          avatar_url: userDetail.avatar_url,
          status: userDetail.status as UserStatus,
          created_at: userDetail.created_at,
          updated_at: userDetail.updated_at,
          last_login_at: userDetail.last_login_at,
          bio: userDetail.bio,
          locale: userDetail.locale,
          roles: userDetail.roles || [],
          permissions: userDetail.permissions || [],
        })
      } catch (error) {
        console.error('Failed to fetch user profile:', error)
        Message.error('获取用户信息失败')
        throw error
      }
    },

    async initiateOAuth(provider: string, redirectTo?: string): Promise<string> {
      try {
        const response = await authApi.initiateOAuth(provider, {
          redirect_to: redirectTo,
        })

        return response.data.auth_url
      } catch (error) {
        Message.error(resolveAuthErrorMessage(error, 'OAuth 初始化失败'))
        throw error
      }
    },

    async handleOAuthCallback(provider: string, callbackData: OAuthCallbackRequest): Promise<string> {
      this.loading = true
      const userStore = useUserStore()

      try {
        const response = await authApi.handleOAuthCallback(provider, callbackData)

        if (!response.data.access_token || !response.data.user_id) {
          throw new Error('OAuth login response is incomplete')
        }

        userStore.setAuthData({
          accessToken: response.data.access_token,
        })

        await this.fetchUserProfile(response.data.user_id)

        this.loginType = 'oauth'
        Message.success('登录成功')
        return resolveAdminPostLoginRoute(userStore.permissions)
      } catch (error) {
        userStore.reset()
        console.error('OAuth callback error:', error)
        Message.error(resolveAuthErrorMessage(error, 'OAuth 登录失败'))
        throw error
      } finally {
        this.loading = false
      }
    },

    async logout(): Promise<void> {
      const userStore = useUserStore()

      try {
        await userStore.logout()
      } finally {
        this.loginType = null
      }
    },

    async bootstrapSession(): Promise<boolean> {
      const userStore = useUserStore()
      const sessionEpoch = userStore.sessionEpoch
      try {
        const response = await authApi.refreshToken()
        if (userStore.sessionEpoch !== sessionEpoch) return false
        userStore.setAuthData({ accessToken: response.data.access_token })
        await this.fetchUserProfile(response.data.user_id)
        return true
      } catch (_error) {
        userStore.reset()
        return false
      }
    },
  },
})

function isTwoFactorChallenge(
  data: LoginResponseData | LoginChallengeResponseData
): data is LoginChallengeResponseData {
  return 'requires_totp' in data && data.requires_totp === true
}
