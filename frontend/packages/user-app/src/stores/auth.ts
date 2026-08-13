import { defineStore } from 'pinia'
import { Message } from '@arco-design/web-vue'
import {
  BrowserSessionEndedError,
  browserSessionBroker,
  isTerminalBrowserSessionFailure,
  resolveAuthErrorMessage,
  useUserStore,
} from '@paigram/shared-components'
import { authApi, profileApi } from '@/api'
import type {
  LoginEmailRequest,
  LoginChallengeResponseData,
  RegisterEmailRequest,
  RegisterEmailResponse,
  OAuthCallbackRequest,
  LoginResponseData,
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

export interface OAuthCallbackResult {
  status: 'login' | 'bound'
}

interface EmailAuthenticationOutcome {
  result: LoginWithEmailResult
  session?: BrowserSessionSnapshot
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

        const response = await profileApi.getProfile(userId)
        if (!browserSessionBroker.isUserSessionCurrent(userId)) throw new BrowserSessionEndedError()

        const profile = response.data

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

    async refreshToken(): Promise<void> {
      const userStore = useUserStore()
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
        if (!browserSessionBroker.isUserSessionCurrent(session.userId)) throw new BrowserSessionEndedError()
      } catch (error) {
        if (browserSessionBroker.isEnded()) userStore.reset()
        throw error
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

    async handleOAuthCallback(
      provider: string,
      callbackData: OAuthCallbackRequest,
      purpose: 'login' | 'bind' = 'login'
    ): Promise<OAuthCallbackResult> {
      this.loading = true
      const userStore = useUserStore()
      let attemptedAccessToken = ''
      let attemptedSession: BrowserSessionSnapshot | undefined

      try {
        if (purpose === 'bind') {
          const response = await authApi.handleOAuthCallback(provider, callbackData)
          if (!response.data.bound || response.data.purpose !== 'bind_login_method') {
            throw new Error('OAuth callback did not complete account binding')
          }
          Message.success('第三方账号绑定成功')
          return { status: 'bound' }
        }

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
        return { status: 'login' }
      } catch (error) {
        if (attemptedSession) await rollbackFailedAuthentication(attemptedSession)
        if (purpose === 'login' && attemptedAccessToken && userStore.token === attemptedAccessToken) userStore.reset()
        console.error('OAuth callback error:', error)
        Message.error(resolveAuthErrorMessage(error, 'OAuth 登录失败'))
        throw error
      } finally {
        this.loading = false
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
