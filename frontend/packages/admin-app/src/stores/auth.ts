import { defineStore } from 'pinia'
import { Message } from '@arco-design/web-vue'
import {
  BrowserSessionEndedError,
  browserSessionBroker,
  isTerminalBrowserSessionFailure,
  resolveAuthErrorMessage,
  useUserStore,
} from '@paigram/shared-components'
import { authApi, userApi } from '@/api'
import type {
  LoginChallengeResponseData,
  LoginEmailRequest,
  LoginResponseData,
  OAuthCallbackRequest,
  UserStatus,
  BrowserSessionSnapshot,
} from '@paigram/shared-components'

interface AuthState {
  loading: boolean
  loginType: 'email' | 'oauth' | 'telegram' | null
}

export interface LoginWithEmailResult {
  status: 'success' | 'requires_totp'
  message?: string
}

interface EmailAuthenticationOutcome {
  result: LoginWithEmailResult
  session?: BrowserSessionSnapshot
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
      let attemptedAccessToken = ''
      let attemptedSession: BrowserSessionSnapshot | undefined

      try {
        const outcome = await browserSessionBroker.authenticate<EmailAuthenticationOutcome>(async () => {
          const response = await authApi.login(credentials)

          if (isTwoFactorChallenge(response.data)) {
            return {
              value: {
                result: {
                  status: 'requires_totp' as const,
                  message: response.data.message,
                },
              },
            }
          }

          attemptedAccessToken = response.data.access_token
          userStore.setAuthData({ accessToken: attemptedAccessToken })
          const session = { accessToken: attemptedAccessToken, userId: response.data.user_id }
          attemptedSession = session

          return {
            value: { result: { status: 'success' as const }, session },
            session,
          }
        })

        if (outcome.session) {
          await this.fetchUserProfile(outcome.session.userId)
          if (!browserSessionBroker.isUserSessionCurrent(outcome.session.userId)) throw new BrowserSessionEndedError()
          this.loginType = 'email'
          Message.success('登录成功')
        }
        return outcome.result
      } catch (error: unknown) {
        if (attemptedSession) await rollbackFailedAuthentication(attemptedSession)
        if (attemptedAccessToken && userStore.token === attemptedAccessToken) userStore.reset()
        Message.error(resolveAuthErrorMessage(error, '登录失败'))
        throw error
      } finally {
        this.loading = false
      }
    },

    async fetchUserProfile(userId: number): Promise<void> {
      const userStore = useUserStore()

      try {
        if (!Number.isSafeInteger(userId) || userId <= 0) {
          throw new Error('User ID not found')
        }

        const response = await userApi.getDetail(userId)
        if (!browserSessionBroker.isUserSessionCurrent(userId)) throw new BrowserSessionEndedError()
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
      let attemptedAccessToken = ''
      let attemptedSession: BrowserSessionSnapshot | undefined

      try {
        const session = await browserSessionBroker.authenticate(async () => {
          const response = await authApi.handleOAuthCallback(provider, callbackData)
          if (!response.data.access_token || !response.data.user_id) {
            throw new Error('OAuth login response is incomplete')
          }
          attemptedAccessToken = response.data.access_token
          userStore.setAuthData({ accessToken: attemptedAccessToken })
          const session = { accessToken: attemptedAccessToken, userId: response.data.user_id }
          attemptedSession = session
          return {
            value: session,
            session,
          }
        })
        await this.fetchUserProfile(session.userId)
        if (!browserSessionBroker.isUserSessionCurrent(session.userId)) throw new BrowserSessionEndedError()

        this.loginType = 'oauth'
        Message.success('登录成功')
        return resolveAdminPostLoginRoute(userStore.permissions)
      } catch (error) {
        if (attemptedSession) await rollbackFailedAuthentication(attemptedSession)
        if (attemptedAccessToken && userStore.token === attemptedAccessToken) userStore.reset()
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
      if (browserSessionBroker.isEnded()) {
        if (userStore.token || userStore.userInfo) userStore.reset()
        return false
      }
      const sessionEpoch = userStore.sessionEpoch
      try {
        const session = await browserSessionBroker.refresh(
          async () => {
            const response = await authApi.refreshToken()
            return { accessToken: response.data.access_token, userId: response.data.user_id }
          },
          {
            commit: (refreshed) => {
              if (userStore.sessionEpoch !== sessionEpoch) throw new BrowserSessionEndedError()
              userStore.setAuthData({ accessToken: refreshed.accessToken })
            },
            onFailure: (error) => {
              if (isTerminalBrowserSessionFailure(error)) browserSessionBroker.invalidate()
            },
          }
        )
        if (userStore.sessionEpoch !== sessionEpoch) return false
        await this.fetchUserProfile(session.userId)
        if (!browserSessionBroker.isUserSessionCurrent(session.userId)) {
          userStore.reset()
          return false
        }
        return true
      } catch {
        if (browserSessionBroker.isEnded()) userStore.reset()
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

async function rollbackFailedAuthentication(session: BrowserSessionSnapshot): Promise<void> {
  try {
    await browserSessionBroker.logoutIfCurrent(session, async () => {
      await authApi.logout()
    })
  } catch (error) {
    console.error('Failed to revoke incomplete browser authentication:', error)
  }
}
