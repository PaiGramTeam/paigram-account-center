import axios from 'axios'
import { Message } from '@arco-design/web-vue'
import type { AxiosInstance, AxiosRequestConfig, AxiosResponse } from 'axios'
import type { ApiResponse, ApiError } from './types'

export interface NormalizedApiError {
  error?: string
  code?: string
  message?: string
  details?: Record<string, unknown>
}

export interface RequestOptions extends AxiosRequestConfig {
  skipErrorToast?: boolean
}

export interface RequestConfig {
  baseURL?: string
  timeout?: number
  getToken?: () => string
  getRefreshToken?: () => string
  setAuthData?: (data: { accessToken: string; refreshToken: string }) => void
  onUnauthorized?: () => void
}

// 创建 axios 实例工厂函数
export function createRequest(config: RequestConfig = {}) {
  const {
    baseURL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api/v1',
    timeout = 30000,
    getToken,
    getRefreshToken,
    setAuthData,
    onUnauthorized,
  } = config

  const instance: AxiosInstance = axios.create({
    baseURL,
    timeout,
    headers: {
      'Content-Type': 'application/json',
    },
  })

  // 请求拦截器
  instance.interceptors.request.use(
    (config) => {
      // 添加 token 到请求头
      const token = getToken?.()
      if (token && config.headers) {
        config.headers.Authorization = `Bearer ${token}`
      }

      return config
    },
    (error) => {
      return Promise.reject(error)
    }
  )

  // 响应拦截器
  instance.interceptors.response.use(
    (response: AxiosResponse<ApiResponse>) => {
      // 直接返回响应数据
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      return response.data as any
    },
    async (error) => {
      const { response, config } = error
      const requestConfig = (config || {}) as RequestOptions

      if (response) {
        const { status, data } = response

        // 处理 401 未授权错误
        if (status === 401 && config.url !== '/auth/refresh') {
          const refreshToken = getRefreshToken?.()

          // 尝试刷新 token
          if (refreshToken && setAuthData) {
            try {
              const refreshResponse = await instance.post('/auth/refresh', {
                refresh_token: refreshToken,
              })

              // 更新 token
              setAuthData({
                accessToken: refreshResponse.data.access_token,
                refreshToken: refreshResponse.data.refresh_token,
              })

              // 重试原请求
              config.headers.Authorization = `Bearer ${refreshResponse.data.access_token}`
              return instance(config)
            } catch (refreshError) {
              // 刷新失败，触发未授权回调
              onUnauthorized?.()
              return Promise.reject(refreshError)
            }
          } else {
            // 没有 refresh token，触发未授权回调
            onUnauthorized?.()
          }
        }

        // 处理其他错误
        const errorData = normalizeApiError(data as ApiError)
        const errorMessage = errorData.message || errorData.error || getDefaultErrorMessage(status)

        if (!requestConfig.skipErrorToast) {
          Message.error(errorMessage)
        }
        return Promise.reject(errorData)
      }

      // 网络错误或超时
      if (!requestConfig.skipErrorToast) {
        Message.error('网络连接异常，请稍后重试')
      }
      return Promise.reject(error)
    }
  )

  // 封装请求方法
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

// 获取默认错误消息
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
