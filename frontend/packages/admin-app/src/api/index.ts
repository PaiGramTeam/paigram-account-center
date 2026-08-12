import {
  createRequest,
  createAuthApi,
  createUserApi,
  createProfileApi,
  createSecurityApi,
  createRoleApi,
  createPermissionApi,
  configureUserLogout,
  createAdminPlatformAccountsApi,
  createAdminSystemApi,
  createAdminServicesApi,
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
  getSessionEpoch: () => useUserStore().sessionEpoch,
  setAuthData: (data: { accessToken: string }) => {
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
configureUserLogout(async () => {
  await authApi.logout()
})
export const userApi = createUserApi(request)
export const profileApi = createProfileApi(request)
export const securityApi = createSecurityApi(request)
export const roleApi = createRoleApi(request)
export const permissionApi = createPermissionApi(request)
export const adminPlatformAccountsApi = createAdminPlatformAccountsApi(request)
export const adminSystemApi = createAdminSystemApi(request)
export const adminServicesApi = createAdminServicesApi(request)
