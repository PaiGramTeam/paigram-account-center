import {
  createRequest,
  createAuthApi,
  createUserApi,
  createProfileApi,
  createSecurityApi,
  configureUserLogout,
  createPlatformAccountsApi,
} from '@paigram/shared-components'
import { useUserStore } from '@paigram/shared-components'
import router from '@/routes'

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
    userStore.reset()
    void router.push('/login')
  },
})

export const authApi = createAuthApi(request)
configureUserLogout(async (token) => {
  await authApi.logout({ token })
})
export const userApi = createUserApi(request)
export const profileApi = createProfileApi(request)
export const securityApi = createSecurityApi(request)
export const platformAccountsApi = createPlatformAccountsApi(request)
