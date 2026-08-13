import {
  createRequest,
  createAuthApi,
  createUserApi,
  createProfileApi,
  createSecurityApi,
  configureUserLogout,
  browserSessionBroker,
  isTerminalBrowserSessionFailure,
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
  getSessionEpoch: () => useUserStore().sessionEpoch,
  getSessionId: () => browserSessionBroker.currentSessionId(),
  setAuthData: (data: { accessToken: string }) => {
    const userStore = useUserStore()
    userStore.setAuthData(data)
  },
  coordinateRefresh: (refresh, options) =>
    browserSessionBroker.refresh(refresh, {
      rejectedAccessToken: options.rejectedAccessToken,
      commit: options.commit,
      onFailure: (error) => {
        if (isTerminalBrowserSessionFailure(error)) browserSessionBroker.invalidate()
      },
    }),
  onUnauthorized: async (rejectedAccessToken) => {
    const ended =
      browserSessionBroker.isEnded() || (await browserSessionBroker.invalidateIfCurrent(rejectedAccessToken))
    if (!ended) return
    const userStore = useUserStore()
    if (userStore.token && userStore.token !== rejectedAccessToken) return
    userStore.reset()
    const redirect = router.currentRoute.value.path
    void router.push({ path: '/login', query: redirect === '/login' ? undefined : { redirect } })
  },
})

export const authApi = createAuthApi(request)
configureUserLogout(async () => {
  await browserSessionBroker.logout(async () => {
    await authApi.logout()
  })
})
export const userApi = createUserApi(request)
export const profileApi = createProfileApi(request)
export const securityApi = createSecurityApi(request)
export const platformAccountsApi = createPlatformAccountsApi(request)
