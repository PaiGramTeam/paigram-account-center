import { createApp } from 'vue'
import App from './App.vue'
import pinia from './stores'
import router from './routes'
import {
  enableMocking,
  setupRouterGuard,
  setupI18n,
  useUserStore,
  usePermissionStore,
  useAppStore,
} from '@paigram/shared-components'
import type { RouterGuardConfig } from '@paigram/shared-components'
import { resolveAdminPostLoginRoute, useAuthStore } from '@/stores/auth'
import { setupBrowserSessionSync } from '@/stores/session-sync'

import './style.css'
import '@arco-design/web-vue/es/message/style/index.css'
import '@arco-design/web-vue/es/notification/style/index.css'

async function bootstrap(): Promise<void> {
  await enableMocking()

  const app = createApp(App)

  app.use(pinia)

  const appStore = useAppStore()
  appStore.initTheme()

  setupI18n(app)

  const authStore = useAuthStore()
  const startupLocation = `${window.location.pathname}${window.location.search}${window.location.hash}`
  const publicStartup = router.resolve(startupLocation).meta.requiresAuth === false
  if (!publicStartup) await authStore.bootstrapSession()

  const routerGuardConfig: RouterGuardConfig = {
    getUserStore: () => useUserStore(),
    getPermissionStore: () => usePermissionStore(),
    whiteList: ['/login', '/404', '/403'],
  }
  setupRouterGuard(router, routerGuardConfig)

  app.use(router)

  app.mount('#app')
  setupBrowserSessionSync()
  if (publicStartup) {
    void authStore.bootstrapSession().then(async (restored) => {
      await router.isReady()
      if (restored && router.currentRoute.value.path === '/login') {
        await router.replace(resolveAdminPostLoginRoute(useUserStore().permissions))
      }
    })
  }
}

void bootstrap()
