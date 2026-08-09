<template>
  <MainLayout
    :menu-items="menuItems"
    :show-sidebar="true"
    :show-search="true"
    :show-breadcrumb="true"
    :show-footer="true"
    @search="handleSearch"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { RouteRecordNormalized, RouteRecordRaw } from 'vue-router'
import { MainLayout, type MenuItem } from '@paigram/shared-components'

const router = useRouter()

// 获取菜单项
const menuItems = computed<MenuItem[]>(() => {
  // 从路由配置生成菜单
  return generateMenuFromRoutes(router.getRoutes())
})

// 从路由生成菜单
function generateMenuFromRoutes(routes: (RouteRecordNormalized | RouteRecordRaw)[]): MenuItem[] {
  const menu: MenuItem[] = []

  routes.forEach((route) => {
    // 过滤掉隐藏的路由
    if (route.meta?.hidden || route.meta?.hideInMenu) return

    // 过滤掉没有名称的路由
    if (!route.name) return

    // 过滤掉不需要认证的路由（除了首页）
    if (!route.meta?.requiresAuth && route.name !== 'Home') return

    // 过滤掉 Layout 相关的路由
    if (
      route.name === 'Layout' ||
      (typeof route.name === 'string' && route.name.endsWith('Layout') && route.name !== 'DashboardLayout')
    ) {
      // 将 Layout 的子路由提取出来
      if (route.children && route.children.length > 0) {
        menu.push(...generateMenuFromRoutes(Array.from(route.children)))
      }
      return
    }

    const menuItem: MenuItem = {
      path: route.path,
      name: route.name as string,
      meta: route.meta || {},
    }

    if (route.children && route.children.length > 0) {
      const childMenus = generateMenuFromRoutes(Array.from(route.children))
      if (childMenus.length > 0) {
        menuItem.children = childMenus
      }
    }

    menu.push(menuItem)
  })

  return menu.sort((a, b) => (a.meta.sort || 0) - (b.meta.sort || 0))
}

// 处理搜索
const handleSearch = (value: string) => {
  console.log('Search:', value)
  // TODO: 实现搜索功能
}
</script>
