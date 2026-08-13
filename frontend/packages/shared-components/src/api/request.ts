import axios from 'axios'
import { Message } from '@arco-design/web-vue'
import type { AxiosInstance, AxiosRequestConfig, AxiosResponse } from 'axios'
import type { ApiResponse, ApiError } from './types'
import { AuthRefreshCoordinator } from './auth-refresh-coordinator'

export interface NormalizedApiError {
  error?: string
  code?: string
  message?: string
  status?: number
  details?: Record<string, unknown>
}

export interface RequestOptions extends AxiosRequestConfig {
  skipErrorToast?: boolean
  authRetryAttempted?: boolean
  browserSessionId?: string
}

export class BrowserSessionChangedError extends Error {
  constructor() {
    super('Browser session changed while the request was in flight')
    this.name = 'BrowserSessionChangedError'
  }
}

export interface RequestConfig {
  baseURL?: string
  timeout?: number
  getToken?: () => string
  getSessionId?: () => string
  getSessionEpoch?: () => number
  setAuthData?: (data: { accessToken: string }) => void
  coordinateRefresh?: (
    refresh: () => Promise<{ accessToken: string; userId: number }>,
    options: {
      rejectedAccessToken: string
      commit: (session: { accessToken: string; userId: number }) => void
    }
  ) => Promise<{ accessToken: string; userId: number }>
  onUnauthorized?: (rejectedAccessToken: string) => void | Promise<void>
}

export function createRequest(config: RequestConfig = {}) {
  const {
    baseURL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api/v1',
    timeout = 30000,
    getToken,
    getSessionId,
    getSessionEpoch,
    setAuthData,
    coordinateRefresh,
    onUnauthorized,
  } = config

  const instance: AxiosInstance = axios.create({
    baseURL,
    timeout,
    withCredentials: true,
    headers: {
      'Content-Type': 'application/json',
    },
  })

  const refreshCoordinator = new AuthRefreshCoordinator<{
    accessToken: string
  }>()

  instance.interceptors.request.use(
    (config) => {
      const requestConfig = config as typeof config & {
        authRetryAttempted?: boolean
        browserSessionId?: string
      }
      if (requestConfig.browserSessionId === undefined) {
        requestConfig.browserSessionId = getSessionId?.() ?? ''
      } else {
        assertRequestSessionCurrent(requestConfig.browserSessionId, getSessionId)
      }
      const token = getToken?.()
      if (token && !requestConfig.authRetryAttempted) {
        requestConfig.headers.Authorization = `Bearer ${token}`
      }

      return requestConfig
    },
    (error) => {
      return Promise.reject(error)
    }
  )

  instance.interceptors.response.use(
    (response: AxiosResponse<ApiResponse>) => {
      const requestConfig = response.config as RequestOptions
      assertRequestSessionCurrent(requestConfig.browserSessionId, getSessionId)
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      return response.data as any
    },
    async (error) => {
      const { response, config } = error
      const requestConfig = (config || {}) as RequestOptions
      assertRequestSessionCurrent(requestConfig.browserSessionId, getSessionId)

      if (response) {
        const { status, data } = response
        const isAuthenticationRequest = requestConfig.url?.startsWith('/auth/') === true

        if (status === 401 && !isAuthenticationRequest && !requestConfig.authRetryAttempted) {
          assertRequestSessionCurrent(requestConfig.browserSessionId, getSessionId)
          if (setAuthData) {
            try {
              requestConfig.authRetryAttempted = true
              const sessionEpoch = getSessionEpoch?.()
              const rejectedAccessToken = bearerToken(requestConfig.headers?.Authorization)
              const refresh = async () => {
                const refreshResponse = await instance.post('/auth/refresh', undefined, {
                  skipErrorToast: true,
                } as RequestOptions)
                const refreshed = {
                  accessToken: refreshResponse.data.access_token,
                  userId: refreshResponse.data.user_id,
                }

                if (sessionEpoch !== undefined && getSessionEpoch?.() !== sessionEpoch) {
                  throw new Error('Session changed while refreshing')
                }

                return refreshed
              }
              const authData = await refreshCoordinator.run(
                async () => {
                  if (!coordinateRefresh) return refresh()
                  return coordinateRefresh(refresh, {
                    rejectedAccessToken,
                    commit: (session) => setAuthData(session),
                  })
                },
                async (refreshError) => {
                  const status = (refreshError as NormalizedApiError | null)?.status
                  if (status === 401 || status === 403) await onUnauthorized?.(rejectedAccessToken)
                }
              )

              if (sessionEpoch !== undefined && getSessionEpoch?.() !== sessionEpoch) {
                throw new Error('Session changed while refreshing')
              }
              assertRequestSessionCurrent(requestConfig.browserSessionId, getSessionId)
              if (!coordinateRefresh) setAuthData(authData)

              requestConfig.headers!.Authorization = `Bearer ${authData.accessToken}`
              return instance(requestConfig)
            } catch (refreshError) {
              return Promise.reject(refreshError)
            }
          } else {
            await onUnauthorized?.(bearerToken(requestConfig.headers?.Authorization))
          }
        } else if (status === 401 && requestConfig.authRetryAttempted) {
          await onUnauthorized?.(bearerToken(requestConfig.headers?.Authorization))
        }

        const errorData = normalizeApiError(data as ApiError)
        errorData.status = status
        const errorMessage = errorData.message || errorData.error || getDefaultErrorMessage(status)

        if (!requestConfig.skipErrorToast) {
          Message.error(errorMessage)
        }
        return Promise.reject(errorData)
      }

      if (!requestConfig.skipErrorToast) {
        Message.error('网络连接异常，请稍后重试')
      }
      return Promise.reject(error)
    }
  )

  return {
    instance,

    get<T = unknown>(url: string, config?: RequestOptions): Promise<ApiResponse<T>> {
      return instance.get(url, config)
    },

    post<T = unknown>(url: string, data?: unknown, config?: RequestOptions): Promise<ApiResponse<T>> {
      return instance.post(url, data, config)
    },

    put<T = unknown>(url: string, data?: unknown, config?: RequestOptions): Promise<ApiResponse<T>> {
      return instance.put(url, data, config)
    },

    patch<T = unknown>(url: string, data?: unknown, config?: RequestOptions): Promise<ApiResponse<T>> {
      return instance.patch(url, data, config)
    },

    delete<T = unknown>(url: string, config?: RequestOptions): Promise<ApiResponse<T>> {
      return instance.delete(url, config)
    },
  }
}

function bearerToken(value: unknown): string {
  if (typeof value !== 'string') return ''
  const match = /^Bearer\s+(.+)$/i.exec(value.trim())
  return match?.[1]?.trim() ?? ''
}

function assertRequestSessionCurrent(expected: string | undefined, getSessionId?: () => string): void {
  if (getSessionId && getSessionId() !== (expected ?? '')) throw new BrowserSessionChangedError()
}

export function normalizeApiError(error: ApiError | undefined): NormalizedApiError {
  if (!error) {
    return { message: '请求失败' }
  }

  if (typeof error.error === 'string') {
    return {
      code: error.code,
      error: error.error,
      message: error.message || error.error,
      details: error.details,
    }
  }

  if (error.error && typeof error.error === 'object') {
    return {
      code: error.error.code || error.code,
      error: error.error.message || error.message,
      message: error.error.message || error.message,
      details: error.error.details || error.details,
    }
  }

  return {
    code: error.code,
    error: error.message,
    message: error.message,
    details: error.details,
  }
}

function getDefaultErrorMessage(status: number): string {
  switch (status) {
    case 400:
      return '请求参数错误'
    case 403:
      return '没有权限访问'
    case 404:
      return '请求资源不存在'
    case 409:
      return '资源冲突'
    case 500:
      return '服务器错误'
    case 502:
      return '网关错误'
    case 503:
      return '服务暂不可用'
    default:
      return '请求失败'
  }
}
