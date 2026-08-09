<template>
  <BasicLayout
    :menu-items="menuItems"
    :show-notifications="true"
    app-title="Paigram Admin"
    app-title-short="PA"
    :collapsible="true"
    :default-collapsed="false"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import { useRouter } from 'vue-router'
import { BasicLayout, useUserStore } from '@paigram/shared-components'

const router = useRouter()
const userStore = useUserStore()

// Build the administrator menu from routes visible to the current user.
const menuItems = computed<RouteRecordRaw[]>(() => {
  return filterRoutes([...router.options.routes])
})

function filterRoutes(routes: RouteRecordRaw[]): RouteRecordRaw[] {
  return routes
    .map((route) => ({ ...route }))
    .filter((route) => {
      console.log(`检查路由: ${route.name as string}, hideInMenu: ${route.meta?.hideInMenu}`)

      if (route.meta?.hideInMenu) {
        return false
      }

      if (route.meta?.permissions && Array.isArray(route.meta.permissions)) {
        const userPermissions = userStore.userInfo?.permissions || []
        if (userPermissions.length > 0) {
          const hasPermission = (route.meta.permissions as string[]).some((permission: string) =>
            userStore.hasPermission(permission)
          )
          if (!hasPermission) {
            return false
          }
        }
      }

      if (route.children && route.children.length) {
        route.children = filterRoutes(route.children)
        if (route.redirect || route.component) {
          return true
        }
        return route.children.length > 0
      }
      return true
    })
}
</script>
