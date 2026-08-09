import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import type { MenuItem } from '../types'
import { useUserStore } from '../stores/user'
import { usePermissionStore } from '../stores/permission'

/**
 * 生成菜单项数组
 */
export const useMenuGeneration = () => {
  const router = useRouter()
  const userStore = useUserStore()
  const permissionStore = usePermissionStore()

  /**
   * 检查是否有权限访问路由
   */
  const hasPermission = (route: RouteRecordRaw): boolean => {
    const { meta } = route
    if (!meta) return true

    // 检查角色权限
    const roles = meta.roles as string[] | undefined
    if (roles && roles.length > 0) {
      const hasRole = roles.some((role: string) => userStore.hasRole(role))
      if (!hasRole) return false
    }

    // 检查具体权限
    const permissions = meta.permissions as string[] | undefined
    if (permissions && permissions.length > 0) {
      const hasPerm = permissions.some((permission: string) => userStore.hasPermission(permission))
      if (!hasPerm) return false
    }

    return true
  }

  /**
   * 将路由转换为菜单项
   */
  const routeToMenuItem = (route: RouteRecordRaw, basePath = ''): MenuItem | null => {
    // 隐藏的路由不生成菜单
    if (route.meta?.hideInMenu) return null

    // 检查权限
    if (!hasPermission(route)) return null

    const path = basePath ? `${basePath}/${route.path}` : route.path
    const normalizedPath = path.replace(/\/+/g, '/') // 移除重复的斜杠

    const menuItem: MenuItem = {
      path: normalizedPath,
      name: route.name as string,
      meta: {
        ...route.meta,
        title: (route.meta?.title as string) || (route.name as string),
        locale: route.meta?.locale as string | undefined,
      },
    }

    // 处理子路由
    if (route.children && route.children.length > 0) {
      const children: MenuItem[] = []
      for (const child of route.children) {
        const childItem = routeToMenuItem(child, normalizedPath)
        if (childItem) {
          children.push(childItem)
        }
      }

      // 如果有子菜单项，添加到菜单项中
      if (children.length > 0) {
        // 如果设置了 hideChildrenInMenu，则不显示子菜单
        if (!route.meta?.hideChildrenInMenu) {
          menuItem.children = children
        }
      } else {
        // 如果没有可显示的子菜单，且当前路由没有组件，则不显示此菜单
        if (!route.component) {
          return null
        }
      }
    }

    return menuItem
  }

  /**
   * 根据路由生成菜单
   */
  const generateMenuFromRoutes = (routes: RouteRecordRaw[]): MenuItem[] => {
    const menuItems: MenuItem[] = []

    for (const route of routes) {
      const menuItem = routeToMenuItem(route)
      if (menuItem) {
        menuItems.push(menuItem)
      }
    }

    // 根据 order 字段排序
    return menuItems.sort((a, b) => {
      const orderA = a.meta?.order ?? 0
      const orderB = b.meta?.order ?? 0
      return orderB - orderA // 值越高越靠前
    })
  }

  /**
   * 获取所有菜单项
   */
  const menuItems = computed(() => {
    const routes = router.getRoutes()
    // 过滤出顶级路由（通常是 Layout 路由的子路由）
    const topLevelRoutes =
      routes
        .filter((route) => route.meta?.requiresAuth !== false) // 排除不需要认证的路由
        .filter((route) => route.path === '/' || route.path.indexOf('/') === 0) // 顶级路由
        .find((route) => route.name === 'Layout')?.children || []

    return generateMenuFromRoutes(topLevelRoutes as RouteRecordRaw[])
  })

  /**
   * 获取动态路由菜单项
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
