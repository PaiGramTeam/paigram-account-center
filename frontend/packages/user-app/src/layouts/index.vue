<template>
  <MainLayout
    :menu-items="menuItems"
    :show-sidebar="true"
    :show-search="false"
    :show-breadcrumb="true"
    :show-footer="true"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { RouteRecordNormalized, RouteRecordRaw } from 'vue-router'
import { MainLayout, type MenuItem } from '@paigram/shared-components'

const router = useRouter()

const menuItems = computed<MenuItem[]>(() => {
  return generateMenuFromRoutes(router.getRoutes())
})

function generateMenuFromRoutes(routes: (RouteRecordNormalized | RouteRecordRaw)[]): MenuItem[] {
  const menu: MenuItem[] = []

  routes.forEach((route) => {
    if (route.meta?.hidden || route.meta?.hideInMenu) return

    if (!route.name) return

    if (!route.meta?.requiresAuth && route.name !== 'Home') return

    if (
      route.name === 'Layout' ||
      (typeof route.name === 'string' && route.name.endsWith('Layout') && route.name !== 'DashboardLayout')
    ) {
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
</script>
