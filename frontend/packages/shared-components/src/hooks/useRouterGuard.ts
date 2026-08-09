import type { Router } from 'vue-router'
import type { MenuItem } from '../types'

interface UserStoreForGuard {
  isLogin: boolean
  userInfo: unknown
  fetchUserInfo: () => Promise<void>
  logout: () => Promise<void>
  hasPermission: (permission: string) => boolean
  hasRole: (role: string) => boolean
}

interface PermissionStoreForGuard {
  generateRoutes: (routes: MenuItem[]) => MenuItem[]
}

export interface RouterGuardConfig {
  whiteList?: string[]
  getUserStore: () => UserStoreForGuard
  getPermissionStore?: () => PermissionStoreForGuard
}

export function setupRouterGuard(router: Router, config: RouterGuardConfig) {
  const { whiteList = ['/login', '/register', '/404', '/403'], getUserStore } = config

  // 全局前置守卫
  router.beforeEach(async (to, _from, next) => {
    const userStore = getUserStore()

    // 设置页面标题
    if (to.meta?.title) {
      document.title = `${to.meta.title} - Paigram Account Center`
    }

    // 判断是否登录
    if (userStore.isLogin) {
      if (to.path === '/login') {
        // 已登录且要跳转的页面是登录页
        next({ path: '/' })
      } else {
        // 权限验证
        if (to.meta?.requiresAuth === false) {
          next()
        } else if (to.meta?.permissions || to.meta?.roles) {
          // 检查权限
          const hasPermission = checkPermission(to.meta, userStore)
          if (hasPermission) {
            next()
          } else {
            next('/403')
          }
        } else {
          next()
        }
      }
    } else {
      // 未登录
      if (whiteList.includes(to.path) || to.meta?.requiresAuth === false) {
        // 在免登录白名单，直接进入
        next()
      } else {
        // 其他没有访问权限的页面将重定向到登录页面
        next(`/login?redirect=${to.path}`)
      }
    }
  })

  // 全局后置守卫
  router.afterEach((_to) => {
    // 结束进度条
    // NProgress.done()
  })
}

// 检查权限
function checkPermission(
  meta: { roles?: string[]; permissions?: string[] },
  userStore: { hasRole: (role: string) => boolean; hasPermission: (permission: string) => boolean }
): boolean {
  if (meta.roles && meta.roles.length > 0) {
    return meta.roles.some((role: string) => userStore.hasRole(role))
  }

  if (meta.permissions && meta.permissions.length > 0) {
    return meta.permissions.some((permission: string) => userStore.hasPermission(permission))
  }

  return true
}
