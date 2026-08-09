import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import type { MenuItem } from '../types'
import { useUserStore } from '../stores/user'
import { usePermissionStore } from '../stores/permission'

/**
 */
export const useMenuGeneration = () => {
  const router = useRouter()
  const userStore = useUserStore()
  const permissionStore = usePermissionStore()

  /**
   */
  const hasPermission = (route: RouteRecordRaw): boolean => {
    const { meta } = route
    if (!meta) return true

    const roles = meta.roles as string[] | undefined
    if (roles && roles.length > 0) {
      const hasRole = roles.some((role: string) => userStore.hasRole(role))
      if (!hasRole) return false
    }

    const permissions = meta.permissions as string[] | undefined
    if (permissions && permissions.length > 0) {
      const hasPerm = permissions.some((permission: string) => userStore.hasPermission(permission))
      if (!hasPerm) return false
    }

    return true
  }

  /**
   */
  const routeToMenuItem = (route: RouteRecordRaw, basePath = ''): MenuItem | null => {
    if (route.meta?.hideInMenu) return null

    if (!hasPermission(route)) return null

    const path = basePath ? `${basePath}/${route.path}` : route.path
    const normalizedPath = path.replace(/\/+/g, '/')

    const menuItem: MenuItem = {
      path: normalizedPath,
      name: route.name as string,
      meta: {
        ...route.meta,
        title: (route.meta?.title as string) || (route.name as string),
        locale: route.meta?.locale as string | undefined,
      },
    }

    if (route.children && route.children.length > 0) {
      const children: MenuItem[] = []
      for (const child of route.children) {
        const childItem = routeToMenuItem(child, normalizedPath)
        if (childItem) {
          children.push(childItem)
        }
      }

      if (children.length > 0) {
        if (!route.meta?.hideChildrenInMenu) {
          menuItem.children = children
        }
      } else {
        if (!route.component) {
          return null
        }
      }
    }

    return menuItem
  }

  /**
   */
  const generateMenuFromRoutes = (routes: RouteRecordRaw[]): MenuItem[] => {
    const menuItems: MenuItem[] = []

    for (const route of routes) {
      const menuItem = routeToMenuItem(route)
      if (menuItem) {
        menuItems.push(menuItem)
      }
    }

    return menuItems.sort((a, b) => {
      const orderA = a.meta?.order ?? 0
      const orderB = b.meta?.order ?? 0
      return orderB - orderA
    })
  }

  /**
   */
  const menuItems = computed(() => {
    const routes = router.getRoutes()
    const topLevelRoutes =
      routes
        .filter((route) => route.meta?.requiresAuth !== false)
        .filter((route) => route.path === '/' || route.path.indexOf('/') === 0)
        .find((route) => route.name === 'Layout')?.children || []

    return generateMenuFromRoutes(topLevelRoutes as RouteRecordRaw[])
  })

  /**
   */
  const asyncMenuItems = computed(() => {
    // dynamicRoutes is already MenuItem[], no need to convert
    return permissionStore.dynamicRoutes
  })

  return {
    menuItems,
    asyncMenuItems,
    generateMenuFromRoutes,
    hasPermission,
  }
}
