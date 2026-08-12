import { createApp } from 'vue'
import App from './App.vue'
import router from './routes'
import pinia from './stores'
import {
  setupPermissionDirective,
  setupRouterGuard,
  setupI18n,
  useUserStore,
  usePermissionStore,
  useAppStore,
  enableMocking,
} from '@paigram/shared-components'
import type { RouterGuardConfig } from '@paigram/shared-components'
import { useAuthStore } from '@/stores/auth'

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

  await useAuthStore().bootstrapSession()

  const routerGuardConfig: RouterGuardConfig = {
    getUserStore: () => useUserStore(),
    getPermissionStore: () => usePermissionStore(),
    whiteList: ['/login', '/register', '/404', '/403'],
  }
  setupRouterGuard(router, routerGuardConfig)

  app.use(router)

  setupPermissionDirective(app)

  app.mount('#app')
}

void bootstrap()
