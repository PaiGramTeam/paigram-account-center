import { defineStore } from 'pinia'
import type { MenuItem } from '../types'
import { useUserStore } from './user'

interface PermissionState {
  routes: MenuItem[]
  dynamicRoutes: MenuItem[]
}

export const usePermissionStore = defineStore('permission', {
  state: (): PermissionState => ({
    routes: [],
    dynamicRoutes: [],
  }),

  actions: {
    generateRoutes(routes: MenuItem[]) {
      const userStore = useUserStore()

      const accessedRoutes = this.filterAsyncRoutes(routes, userStore.roles, userStore.permissions)

      this.routes = accessedRoutes
      this.dynamicRoutes = accessedRoutes

      return accessedRoutes
    },

    filterAsyncRoutes(routes: MenuItem[], roles: string[], permissions: string[]): MenuItem[] {
      const res: MenuItem[] = []

      routes.forEach((route) => {
        const tmp = { ...route }
        if (this.hasPermission(tmp, roles, permissions)) {
          if (tmp.children) {
            tmp.children = this.filterAsyncRoutes(tmp.children, roles, permissions)
          }
          res.push(tmp)
        }
      })

      return res
    },

    hasPermission(route: MenuItem, roles: string[], permissions: string[]): boolean {
      if (!route.meta) return true

      if (route.meta.roles && route.meta.roles.length > 0) {
        return roles.some((role) => route.meta.roles!.includes(role))
      }

      if (route.meta.permissions && route.meta.permissions.length > 0) {
        return permissions.some((permission) => route.meta.permissions!.includes(permission))
      }

      return true
    },

    reset() {
      this.routes = []
      this.dynamicRoutes = []
    },
  },
})
