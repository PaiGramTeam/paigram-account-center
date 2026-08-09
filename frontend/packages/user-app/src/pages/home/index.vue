<template>
  <div class="min-h-screen bg-gray-50 dark:bg-gray-900">
    <!-- Header -->
    <Header :navigation-items="navigationItems" />

    <!-- Hero Section -->
    <section class="relative bg-white shadow-sm dark:bg-gray-800">
      <div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
        <div class="py-24 text-center">
          <h1 class="text-4xl font-bold tracking-tight text-gray-900 sm:text-5xl md:text-6xl dark:text-gray-100">
            欢迎来到 <span class="text-blue-500 dark:text-blue-400">Paigram</span>
          </h1>
          <p class="mx-auto mt-6 max-w-2xl text-lg leading-8 text-gray-600 dark:text-gray-300">
            Paigram 账号中心为 PaiGram Bot 系列提供统一的用户认证和管理服务， 让您轻松管理所有相关服务的账号信息。
          </p>
          <div class="mt-10 flex items-center justify-center gap-x-6">
            <a-button type="primary" size="large" @click="handleGetStarted">
              立即开始
              <template #icon>
                <icon-arrow-right />
              </template>
            </a-button>
            <a-button size="large" @click="handleLearnMore"> 了解更多 </a-button>
          </div>
        </div>
      </div>
    </section>

    <!-- Features Section -->
    <section class="py-24">
      <div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
        <div class="text-center">
          <h2 class="text-3xl font-bold tracking-tight text-gray-900 sm:text-4xl dark:text-gray-100">核心功能</h2>
          <p class="mx-auto mt-4 max-w-2xl text-lg text-gray-600 dark:text-gray-300">
            为您提供安全、便捷、高效的账号管理体验
          </p>
        </div>

        <div class="mt-16 grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3">
          <div v-for="feature in features" :key="feature.name" class="group relative">
            <div
              class="flex h-full flex-col rounded-lg bg-white p-8 shadow-sm ring-1 ring-gray-200 transition-all hover:shadow-md dark:bg-gray-800 dark:ring-gray-700"
            >
              <div class="flex h-12 w-12 items-center justify-center rounded-lg bg-blue-100 dark:bg-blue-900">
                <component :is="feature.icon" class="h-6 w-6 text-blue-500 dark:text-blue-400" />
              </div>
              <h3 class="mt-6 text-lg font-semibold text-gray-900 dark:text-gray-100">
                {{ feature.name }}
              </h3>
              <p class="mt-2 flex-1 text-base text-gray-600 dark:text-gray-300">
                {{ feature.description }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- CTA Section -->
    <section class="bg-blue-500 dark:bg-blue-700">
      <div class="mx-auto max-w-7xl px-4 py-16 sm:px-6 lg:px-8">
        <div class="text-center">
          <h2 class="text-3xl font-bold tracking-tight text-white sm:text-4xl">准备好开始了吗？</h2>
          <p class="mx-auto mt-4 max-w-2xl text-lg text-white/90">立即注册账号，体验 Paigram 带来的便捷服务</p>
          <div class="mt-8 flex items-center justify-center gap-x-4">
            <a-button size="large" @click="handleRegister"> 注册账号 </a-button>
            <a-button type="text" size="large" class="!text-white hover:!text-white/80" @click="handleContactUs">
              联系我们
              <template #icon>
                <icon-arrow-right />
              </template>
            </a-button>
          </div>
        </div>
      </div>
    </section>

    <!-- Footer -->
    <Footer />
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import {
  IconLock,
  IconUser,
  IconSync,
  IconSafe,
  IconMobile,
  IconApps,
  IconArrowRight,
} from '@arco-design/web-vue/es/icon'
import { Header, Footer } from '@paigram/shared-components'

const router = useRouter()

const navigationItems = [
  { key: 'home', label: '首页', path: '/' },
  { key: 'features', label: '功能', path: '#features' },
  { key: 'about', label: '关于', path: '#about' },
  { key: 'contact', label: '联系', path: '#contact' },
]

const features = [
  {
    name: '统一认证',
    description: '一个账号，登录所有 PaiGram Bot 系列服务，无需重复注册。',
    icon: IconUser,
  },
  {
    name: '安全可靠',
    description: '采用先进的加密技术和安全措施，确保您的账号信息安全。',
    icon: IconSafe,
  },
  {
    name: '多种登录方式',
    description: '支持邮箱、Telegram、Google 等多种登录方式，灵活便捷。',
    icon: IconApps,
  },
  {
    name: '数据同步',
    description: '实时同步账号数据，确保在不同服务间的信息一致性。',
    icon: IconSync,
  },
  {
    name: '移动适配',
    description: '完美适配移动设备，随时随地管理您的账号信息。',
    icon: IconMobile,
  },
  {
    name: '隐私保护',
    description: '严格遵守隐私保护法规，您的个人信息绝对安全。',
    icon: IconLock,
  },
]

const handleGetStarted = (): void => {
  router.push('/login')
}

const handleLearnMore = (): void => {
  Message.info('功能开发中...')
}

const handleRegister = (): void => {
  router.push('/register')
}

const handleContactUs = (): void => {
  router.push('/contact')
}
</script>

<style scoped></style>
