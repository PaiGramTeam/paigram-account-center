<template>
  <div class="app-sidebar h-full border-r border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900">
    <a-menu
      :selected-keys="selectedKeys"
      :open-keys="openKeys"
      :collapsed="collapsed"
      :accordion="accordion"
      @menu-item-click="handleMenuClick"
      @sub-menu-click="handleSubMenuClick"
      class="h-full"
    >
      <template v-for="item in menuItems" :key="getMenuItemKey(item)">
        <a-sub-menu v-if="item.children && item.children.length > 0" :key="getMenuItemKey(item)">
          <template #icon v-if="item.meta?.icon">
            <component :is="item.meta.icon" />
          </template>
          <template #title>{{ getMenuTitle(item) }}</template>

          <a-menu-item
            v-for="child in item.children"
            :key="getMenuItemKey(child, item.name)"
            :disabled="child.meta?.disabled"
          >
            <template #icon v-if="child.meta?.icon">
              <component :is="child.meta.icon" />
            </template>
            {{ getMenuTitle(child) }}
          </a-menu-item>
        </a-sub-menu>

        <a-menu-item v-else :key="getMenuItemKey(item)" :disabled="item.meta?.disabled">
          <template #icon v-if="item.meta?.icon">
            <component :is="item.meta.icon" />
          </template>
          {{ getMenuTitle(item) }}
        </a-menu-item>
      </template>
    </a-menu>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { MenuItem } from '../../types'

interface Props {
  menuItems?: MenuItem[]
  collapsed?: boolean
  accordion?: boolean
  useRouteMenu?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  collapsed: false,
  accordion: true,
  useRouteMenu: true,
})

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const computedMenuItems = computed(() => {
  if (props.useRouteMenu && !props.menuItems) {
    return []
  }
  return props.menuItems || []
})

const selectedKeys = ref<string[]>([])
const openKeys = ref<string[]>([])

const getMenuTitle = (item: MenuItem): string => {
  if (item.meta?.locale) {
    return t(item.meta.locale)
  }
  return item.meta?.title || item.name || ''
}

const getMenuItemKey = (item: MenuItem, parentName?: string): string => {
  if (item.name) {
    return item.name
  }
  if (parentName && item.path) {
    return `${parentName}-${item.path}`
  }
  return item.path
}

const findMenuPath = (items: MenuItem[], routeName: string, parents: string[] = []): string[] => {
  for (const item of items) {
    const itemKey = getMenuItemKey(item)

    if (item.name === routeName) {
      return [...parents, itemKey]
    }

    if (item.children && item.children.length > 0) {
      const found = findMenuPath(item.children, routeName, [...parents, itemKey])
      if (found.length > 0) {
        return found
      }
    }
  }
  return []
}

watch(
  () => route.name,
  (routeName) => {
    if (!routeName) return

    const menuPath = findMenuPath(computedMenuItems.value, routeName as string)
    if (menuPath.length > 0) {
      const lastKey = menuPath[menuPath.length - 1]
      if (lastKey) {
        selectedKeys.value = [lastKey]
      }
      if (!props.collapsed && menuPath.length > 1) {
        openKeys.value = menuPath.slice(0, -1)
      }
    }
  },
  { immediate: true }
)

const handleMenuClick = (key: string) => {
  const routeExists = router.getRoutes().find((r) => r.name === key)
  if (routeExists) {
    void router.push({ name: key })
  } else {
    void router.push(key)
  }
}

const handleSubMenuClick = (_key: string, newOpenKeys: string[]) => {
  if (!props.collapsed) {
    openKeys.value = newOpenKeys
  }
}
</script>

<style scoped></style>
