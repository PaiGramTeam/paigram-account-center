import { synchronizeBrowserSession, useUserStore } from '@paigram/shared-components'
import router from '@/routes'
import { useAuthStore } from './auth'

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
      await router.replace('/')
    },
    navigateToLogin: async () => {
      const currentPath = router.currentRoute.value.path
      if (currentPath === '/login') return
      await router.replace({ path: '/login', query: { redirect: currentPath } })
    },
    navigateAfterAuthentication: async (principalChanged) => {
      if (principalChanged || router.currentRoute.value.path === '/login') await router.replace('/dashboard')
    },
  })
}
