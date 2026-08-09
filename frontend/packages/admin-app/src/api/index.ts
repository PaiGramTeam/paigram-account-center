import {
  createRequest,
  createAuthApi,
  createUserApi,
  createProfileApi,
  createSecurityApi,
  createRoleApi,
  createPermissionApi,
} from '@paigram/shared-components'
import { useUserStore } from '@paigram/shared-components'
import router from '@/routes'

// 创建配置好的 request 实例
export const request = createRequest({
  baseURL: import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api/v1',
  timeout: 30000,
  getToken: () => {
    const userStore = useUserStore()
    return userStore.token
  },
  getRefreshToken: () => {
    const userStore = useUserStore()
    return userStore.refreshToken
  },
  setAuthData: (data: { accessToken: string; refreshToken: string }) => {
    const userStore = useUserStore()
    userStore.setAuthData(data)
  },
  onUnauthorized: () => {
    const userStore = useUserStore()
    userStore.logout()
    router.push('/login')
  },
})

// 创建 API 实例
export const authApi = createAuthApi(request)
export const userApi = createUserApi(request)
export const profileApi = createProfileApi(request)
export const securityApi = createSecurityApi(request)
export const roleApi = createRoleApi(request)
export const permissionApi = createPermissionApi(request)
