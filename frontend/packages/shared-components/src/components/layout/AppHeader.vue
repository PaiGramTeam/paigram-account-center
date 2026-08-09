<template>
  <div
    class="app-header flex h-16 items-center justify-between border-b border-gray-200 bg-white px-6 shadow-sm dark:border-gray-700 dark:bg-gray-900"
  >
    <!-- 左侧 -->
    <div class="flex items-center">
      <!-- 折叠按钮 -->
      <a-button v-if="showCollapse" type="text" @click="handleToggleCollapse" class="mr-4">
        <template #icon>
          <icon-menu-fold v-if="!collapsed" />
          <icon-menu-unfold v-else />
        </template>
      </a-button>

      <!-- Logo 和标题 -->
      <div class="flex items-center">
        <img v-if="logo" :src="logo" alt="logo" class="mr-3 h-8 w-8" />
        <h1 class="text-xl font-semibold text-gray-900 dark:text-gray-100">{{ title }}</h1>
      </div>
    </div>

    <!-- 右侧 -->
    <div class="flex items-center space-x-4">
      <!-- 搜索 -->
      <a-input-search v-if="showSearch" placeholder="搜索..." class="w-64" @search="handleSearch" />

      <!-- 主题切换 -->
      <a-dropdown trigger="click" @select="handleThemeChange">
        <a-button type="text">
          <template #icon>
            <icon-sun v-if="effectiveTheme === 'light'" />
            <icon-moon v-else-if="effectiveTheme === 'dark'" />
            <icon-computer v-else />
          </template>
        </a-button>
        <template #content>
          <a-doption value="light">
            <template #icon>
              <icon-sun />
            </template>
            浅色{{ theme === 'light' ? ' ✓' : '' }}
          </a-doption>
          <a-doption value="dark">
            <template #icon>
              <icon-moon />
            </template>
            深色{{ theme === 'dark' ? ' ✓' : '' }}
          </a-doption>
          <a-doption value="auto">
            <template #icon>
              <icon-computer />
            </template>
            跟随系统{{ theme === 'auto' ? ' ✓' : '' }}
          </a-doption>
        </template>
      </a-dropdown>

      <!-- 消息通知 -->
      <a-badge :count="9" dot>
        <a-button type="text">
          <template #icon>
            <icon-notification />
          </template>
        </a-button>
      </a-badge>

      <!-- 用户下拉菜单 -->
      <a-dropdown trigger="click" @select="handleUserMenuClick">
        <div class="flex cursor-pointer items-center rounded px-2 py-1 hover:bg-gray-100 dark:hover:bg-gray-800">
          <a-avatar :size="32" class="mr-2">
            <img v-if="userStore.avatar" :src="userStore.avatar" alt="avatar" />
            <icon-user v-else />
          </a-avatar>
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ userStore.nickname || userStore.username }}</span>
        </div>
        <template #content>
          <a-doption value="profile">
            <template #icon>
              <icon-user />
            </template>
            个人中心
          </a-doption>
          <a-doption value="settings">
            <template #icon>
              <icon-settings />
            </template>
            设置
          </a-doption>
          <a-divider class="my-1" />
          <a-doption value="logout">
            <template #icon>
              <icon-export />
            </template>
            退出登录
          </a-doption>
        </template>
      </a-dropdown>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  IconMenuFold,
  IconMenuUnfold,
  IconSun,
  IconMoon,
  IconComputer,
  IconNotification,
  IconUser,
  IconSettings,
  IconExport,
} from '@arco-design/web-vue/es/icon'
import { useAppStore } from '../../stores/app'
import { useUserStore } from '../../stores/user'
import { useRouter } from 'vue-router'

interface Props {
  showCollapse?: boolean
  showSearch?: boolean
}

withDefaults(defineProps<Props>(), {
  showCollapse: true,
  showSearch: true,
})

const emit = defineEmits<{
  search: [value: string]
}>()

const appStore = useAppStore()
const userStore = useUserStore()
const router = useRouter()

const collapsed = computed(() => appStore.collapsed)
const theme = computed(() => appStore.theme)
const effectiveTheme = computed(() => appStore.effectiveTheme)
const title = computed(() => appStore.title)
const logo = computed(() => appStore.logo)

const handleToggleCollapse = () => {
  appStore.toggleCollapsed()
}

const handleThemeChange = (value: string) => {
  appStore.setTheme(value as 'light' | 'dark' | 'auto')
}

const handleSearch = (value: string) => {
  emit('search', value)
}

const handleUserMenuClick = async (value: string) => {
  switch (value) {
    case 'profile':
      router.push('/profile')
      break
    case 'settings':
      router.push('/settings')
      break
    case 'logout':
      await userStore.logout()
      router.push('/login')
      break
  }
}
</script>

<style scoped></style>
