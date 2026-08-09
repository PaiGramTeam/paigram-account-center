import { defineStore } from 'pinia'
import { Message } from '@arco-design/web-vue'
import { resolveAuthErrorMessage, useUserStore } from '@paigram/shared-components'
import { authApi, profileApi } from '@/api'
import type {
  LoginEmailRequest,
  LoginChallengeResponseData,
  RegisterEmailRequest,
  RegisterEmailResponse,
  OAuthCallbackRequest,
  LoginResponseData,
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

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    loading: false,
    loginType: null,
  }),

  actions: {
    // 邮箱密码登录
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

        // 保存认证信息
        userStore.setAuthData({
          accessToken: response.data.access_token,
          refreshToken: response.data.refresh_token,
        })

        // 获取用户信息
        await this.fetchUserProfile(response.data.user_id)

        this.loginType = 'email'
        Message.success('登录成功')
        return { status: 'success' }
      } catch (error: unknown) {
        Message.error(resolveAuthErrorMessage(error, '登录失败'))
        throw error
      } finally {
        this.loading = false
      }
    },

    // 获取用户资料
    async fetchUserProfile(userId?: number): Promise<void> {
      const userStore = useUserStore()

      try {
        const id = userId || userStore.userId
        if (!id) {
          throw new Error('User ID not found')
        }

        const response = await profileApi.getProfile(id)

        const profile = response.data

        // 转换并保存用户信息
        userStore.setUserInfo({
          id: profile.user_id,
          display_name: profile.display_name,
          primary_email: profile.primary_email,
          avatar_url: profile.avatar_url,
          status: profile.status as UserStatus,
          created_at: profile.created_at,
          updated_at: profile.updated_at,
          last_login_at: profile.last_login_at,
          bio: profile.bio,
          locale: profile.locale,
          roles: [],
          permissions: [],
        })

        // No need for additional patch since we already set all the data above
      } catch (error) {
        console.error('Failed to fetch user profile:', error)
        Message.error('获取用户信息失败')
        throw error
      }
    },

    // 邮箱注册
    async registerWithEmail(data: RegisterEmailRequest): Promise<RegisterEmailResponse['data']> {
      this.loading = true

      try {
        const response = await authApi.register(data)

        if (response.data.requires_email_verification) {
          Message.success({
            content: '注册成功！请查看您的邮箱完成验证',
            duration: 5000,
          })
        } else {
          Message.success('注册成功！请登录')
        }

        return response.data
      } catch (error: unknown) {
        Message.error(resolveAuthErrorMessage(error, '注册失败'))
        throw error
      } finally {
        this.loading = false
      }
    },

    // 刷新 Token
    async refreshToken(): Promise<void> {
      const userStore = useUserStore()

      if (!userStore.refreshToken) {
        throw new Error('No refresh token available')
      }

      try {
        const response = await authApi.refreshToken({
          refresh_token: userStore.refreshToken,
        })

        userStore.setAuthData({
          accessToken: response.data.access_token,
          refreshToken: response.data.refresh_token,
        })
      } catch (error) {
        // 刷新失败，清除认证信息
        userStore.logout()
        throw error
      }
    },

    // 登出
    async logout(): Promise<void> {
      const userStore = useUserStore()

      try {
        if (userStore.token) {
          await authApi.logout({ token: userStore.token })
        }
      } catch (error) {
        console.error('Logout error:', error)
      } finally {
        userStore.logout()
        this.loginType = null
      }
    },

    // OAuth 登录
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

    // 处理 OAuth 回调
    async handleOAuthCallback(provider: string, callbackData: OAuthCallbackRequest): Promise<void> {
      this.loading = true
      const userStore = useUserStore()

      try {
        const response = await authApi.handleOAuthCallback(provider, callbackData)

        // 保存认证信息
        userStore.setAuthData({
          accessToken: response.data.access_token,
          refreshToken: response.data.refresh_token,
        })

        // 获取用户信息
        await this.fetchUserProfile(response.data.user_id)

        this.loginType = 'oauth'
        Message.success('登录成功')
      } catch (error) {
        console.error('OAuth callback error:', error)
        Message.error(resolveAuthErrorMessage(error, 'OAuth 登录失败'))
        throw error
      } finally {
        this.loading = false
      }
    },
  },
})

function isTwoFactorChallenge(
  data: LoginResponseData | LoginChallengeResponseData
): data is LoginChallengeResponseData {
  return 'requires_totp' in data && data.requires_totp === true
}
