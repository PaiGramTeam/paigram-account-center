import { synchronizeBrowserSession, useUserStore } from '@paigram/shared-components'
import router from '@/routes'
import { resolveAdminPostLoginRoute, useAuthStore } from './auth'

export function setupBrowserSessionSync(): () => void {
  const userStore = useUserStore()
  const authStore = useAuthStore()
  return synchronizeBrowserSession({
    readState: () => ({
      accessToken: userStore.token,
      authenticated: Boolean(userStore.token || userStore.userInfo),
      profileUserId: userStore.userInfo?.id ?? null,
    }),
    clearSession: () => {
      authStore.$patch({ loginType: null })
      userStore.reset()
    },
    setAccessToken: (accessToken) => userStore.setAuthData({ accessToken }),
    loadProfile: (userId) => authStore.fetchUserProfile(userId),
    navigateBeforePrincipalChange: async () => {
      await router.replace('/login')
    },
    navigateToLogin: async () => {
      if (router.currentRoute.value.path !== '/login') await router.replace('/login')
    },
    navigateAfterAuthentication: async (principalChanged) => {
      if (principalChanged || router.currentRoute.value.path === '/login') {
        await router.replace(resolveAdminPostLoginRoute(userStore.permissions))
      }
    },
  })
}
