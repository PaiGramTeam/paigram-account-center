<template>
  <div class="bg-white shadow-sm dark:bg-gray-900">
    <div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
      <div class="flex items-center justify-between py-3">
        <div class="flex flex-auto items-center">
          <div class="flex items-center">
            <h1 class="mr-8 text-xl font-bold text-gray-900 dark:text-gray-100">Paigram</h1>
          </div>
          <a-menu mode="horizontal" :selected-keys="[activeMenuKey]" @menu-item-click="handleMenuClick">
            <a-menu-item v-for="item in navigationItems" :key="item.key" :disabled="item.disabled">
              <template #icon>
                <component :is="item.icon" v-if="item.icon" />
              </template>
              {{ item.label }}
            </a-menu-item>
          </a-menu>
        </div>
        <a-space>
          <!-- Auth Controls -->
          <template v-if="isLoggedIn">
            <!-- User Dropdown -->
            <a-dropdown trigger="click">
              <a-button type="text" class="flex items-center">
                <a-avatar :size="28" class="mr-2">
                  <template v-if="userStore.avatar">
                    <img :src="userStore.avatar" :alt="userStore.nickname" />
                  </template>
                  <template v-else>
                    <icon-user />
                  </template>
                </a-avatar>
                <span>{{ userStore.nickname || userStore.username }}</span>
                <icon-down class="ml-1" />
              </a-button>
              <template #content>
                <a-doption @click="handleViewProfile">
                  <template #icon>
                    <icon-user />
                  </template>
                  个人资料
                </a-doption>
                <a-doption @click="handleSettings">
                  <template #icon>
                    <icon-settings />
                  </template>
                  账号设置
                </a-doption>
                <a-divider style="margin: 4px 0" />
                <a-doption @click="handleLogout">
                  <template #icon>
                    <icon-export />
                  </template>
                  退出登录
                </a-doption>
              </template>
            </a-dropdown>
          </template>

          <!-- Login Button (for guests) -->
          <a-button v-else size="small" type="primary" @click="handleLogin">
            <template #icon>
              <icon-user />
            </template>
            登录
          </a-button>
        </a-space>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { IconUser, IconDown, IconSettings, IconExport } from '@arco-design/web-vue/es/icon'
import { useUserStore } from '../../stores/user'

interface NavigationItem {
  key: string
  label: string
  path: string
  icon?: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    navigationItems?: NavigationItem[]
  }>(),
  {
    navigationItems: () => [
      { key: 'home', label: '首页', path: '/' },
      { key: 'dashboard', label: '控制台', path: '/dashboard' },
    ],
  }
)

const emit = defineEmits<{
  login: []
  logout: []
  menuClick: [key: string]
}>()

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const isLoggedIn = computed(() => userStore.isLogin)
const activeMenuKey = computed(() => {
  const currentPath = route.path
  const item = props.navigationItems.find((item) => item.path === currentPath)
  return item?.key || ''
})

const handleMenuClick = (key: string): void => {
  const item = props.navigationItems.find((item) => item.key === key)
  if (item && item.path) {
    router.push(item.path)
  }
  emit('menuClick', key)
}

const handleLogin = (): void => {
  emit('login')
  router.push('/login')
}

const handleLogout = async (): Promise<void> => {
  try {
    await userStore.logout()
    Message.success('已退出登录')
    emit('logout')
    router.push('/login')
  } catch (_error) {
    Message.error('退出登录失败')
  }
}

const handleViewProfile = (): void => {
  router.push('/profile')
}

const handleSettings = (): void => {
  router.push('/settings')
}
</script>

<style scoped></style>
