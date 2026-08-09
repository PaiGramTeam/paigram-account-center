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

  router.beforeEach(async (to, _from, next) => {
    const userStore = getUserStore()

    if (to.meta?.title) {
      document.title = `${to.meta.title} - Paigram Account Center`
    }

    if (userStore.isLogin) {
      if (to.path === '/login') {
        next({ path: '/' })
      } else {
        if (to.meta?.requiresAuth === false) {
          next()
        } else if (to.meta?.permissions || to.meta?.roles) {
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
      if (whiteList.includes(to.path) || to.meta?.requiresAuth === false) {
        next()
      } else {
        next(`/login?redirect=${to.path}`)
      }
    }
  })

  router.afterEach((_to) => {
    // NProgress.done()
  })
}

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
