<template>
  <a-layout class="app-layout min-h-screen">
    <!-- Header -->
    <a-layout-header class="h-16 px-0">
      <AppHeader :show-collapse="showSidebar" :show-search="showSearch" @search="handleSearch" />
    </a-layout-header>

    <a-layout>
      <!-- Sidebar -->
      <a-layout-sider
        v-if="showSidebar"
        :collapsed="collapsed"
        :width="220"
        :collapsed-width="64"
        breakpoint="lg"
        class="shadow-sm"
      >
        <AppSidebar :menu-items="menuItems" :collapsed="collapsed" :accordion="accordion" />
      </a-layout-sider>

      <!-- Content -->
      <a-layout>
        <a-layout-content class="relative bg-white dark:bg-gray-800">
          <!-- Breadcrumb -->
          <div v-if="showBreadcrumb" class="border-b bg-white px-6 py-3 dark:bg-gray-900">
            <a-breadcrumb>
              <a-breadcrumb-item v-for="item in breadcrumbItems" :key="item.path">
                {{ item.title }}
              </a-breadcrumb-item>
            </a-breadcrumb>
          </div>

          <!-- Page Content -->
          <div class="overflow-auto p-6" :class="{ 'h-full': !showFooter }">
            <router-view v-slot="{ Component, route: currentRoute }">
              <transition name="fade-slide" mode="out-in" appear>
                <keep-alive v-if="currentRoute.meta?.keepAlive" :include="cachedRoutes">
                  <component :is="Component" :key="currentRoute.path" />
                </keep-alive>
                <component v-else :is="Component" :key="currentRoute.path" />
              </transition>
            </router-view>
          </div>
        </a-layout-content>

        <!-- Footer -->
        <a-layout-footer v-if="showFooter" class="h-12 px-0">
          <AppFooter :copyright="copyright" />
        </a-layout-footer>
      </a-layout>
    </a-layout>
  </a-layout>
</template>

<script setup lang="ts">
import { ref, computed, watch, provide } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '../../stores/app'
import AppHeader from './AppHeader.vue'
import AppSidebar from './AppSidebar.vue'
import AppFooter from './AppFooter.vue'
import type { MenuItem } from '../../types'

interface Props {
  menuItems?: MenuItem[]
  showSidebar?: boolean
  showSearch?: boolean
  showBreadcrumb?: boolean
  showFooter?: boolean
  accordion?: boolean
  copyright?: string
}

const props = withDefaults(defineProps<Props>(), {
  menuItems: () => [],
  showSidebar: true,
  showSearch: true,
  showBreadcrumb: true,
  showFooter: true,
  accordion: true,
})

const emit = defineEmits<{
  search: [value: string]
}>()

const route = useRoute()
const appStore = useAppStore()

const collapsed = computed(() => appStore.collapsed)
const cachedRoutes = ref<string[]>([])
const breadcrumbItems = ref<{ path: string; title: string }[]>([])

// 更新面包屑
watch(
  () => route.path,
  () => {
    // TODO: 根据路由生成面包屑
    breadcrumbItems.value = [
      { path: '/', title: '首页' },
      { path: route.path, title: (route.meta?.title as string) || '页面' },
    ]
  },
  { immediate: true }
)

// 更新缓存路由
watch(
  () => route.name,
  (name) => {
    if (name && route.meta?.keepAlive && typeof name === 'string') {
      if (!cachedRoutes.value.includes(name)) {
        cachedRoutes.value.push(name)
      }
    }
  },
  { immediate: true }
)

const handleSearch = (value: string) => {
  emit('search', value)
}

// 提供布局配置给子组件
provide('layoutConfig', {
  collapsed,
  showSidebar: props.showSidebar,
  showFooter: props.showFooter,
})
</script>

<style scoped></style>
