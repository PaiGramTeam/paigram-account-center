import { createApp } from 'vue'
import App from './App.vue'
import pinia from './stores'
import router from './routes'
import { setupRouterGuard, setupI18n, useUserStore, usePermissionStore, useAppStore } from '@paigram/shared-components'
import type { RouterGuardConfig } from '@paigram/shared-components'

// 导入样式
import './style.css'
import '@arco-design/web-vue/es/message/style/index.css'
import '@arco-design/web-vue/es/notification/style/index.css'

const app = createApp(App)

// 注册状态管理（必须在 router 之前，因为路由守卫使用了 store）
app.use(pinia)

// 初始化主题（在 pinia 注册后）
const appStore = useAppStore()
appStore.initTheme()

// 安装 i18n（必须在使用任何组件前安装）
setupI18n(app)

// 设置路由守卫（必须在 pinia 注册后）
const routerGuardConfig: RouterGuardConfig = {
  getUserStore: () => useUserStore(),
  getPermissionStore: () => usePermissionStore(),
  whiteList: ['/login', '/404', '/403'],
}
setupRouterGuard(router, routerGuardConfig)

// 注册路由
app.use(router)

app.mount('#app')
