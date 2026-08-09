<template>
  <footer class="mt-auto border-t border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-900">
    <div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
      <div class="py-8">
        <!-- 主要内容区 -->
        <div class="grid grid-cols-1 gap-8 md:grid-cols-4">
          <!-- 关于 -->
          <div class="col-span-1 md:col-span-2">
            <h3 class="mb-4 text-lg font-semibold text-gray-900 dark:text-gray-100">{{ brandName }}</h3>
            <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
              {{ description || 'Paigram 账号中心为 PaiGram Bot 系列提供统一的用户认证和管理服务。' }}
            </p>
            <div class="flex space-x-4">
              <a
                v-for="social in socialLinks"
                :key="social.name"
                :href="social.url"
                target="_blank"
                rel="noopener noreferrer"
                class="text-gray-400 hover:text-gray-500 dark:text-gray-500 dark:hover:text-gray-400"
              >
                <span class="sr-only">{{ social.name }}</span>
                <component :is="social.icon" class="h-6 w-6" />
              </a>
            </div>
          </div>

          <!-- 快速链接 -->
          <div>
            <h3 class="mb-4 text-sm font-semibold tracking-wider text-gray-900 uppercase dark:text-gray-100">
              快速链接
            </h3>
            <ul class="space-y-3">
              <li v-for="link in quickLinks" :key="link.name">
                <a
                  :href="link.href"
                  class="text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200"
                  @click.prevent="handleLinkClick(link)"
                >
                  {{ link.name }}
                </a>
              </li>
            </ul>
          </div>

          <!-- 帮助与支持 -->
          <div>
            <h3 class="mb-4 text-sm font-semibold tracking-wider text-gray-900 uppercase dark:text-gray-100">
              帮助与支持
            </h3>
            <ul class="space-y-3">
              <li v-for="link in supportLinks" :key="link.name">
                <a
                  :href="link.href"
                  class="text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200"
                  :target="link.external ? '_blank' : undefined"
                  :rel="link.external ? 'noopener noreferrer' : undefined"
                  @click="!link.external && handleLinkClick(link)"
                >
                  {{ link.name }}
                </a>
              </li>
            </ul>
          </div>
        </div>

        <!-- 底部信息 -->
        <div class="mt-8 border-t border-gray-200 pt-8 dark:border-gray-700">
          <div class="flex flex-col items-center justify-between md:flex-row">
            <p class="text-sm text-gray-500 dark:text-gray-400">
              &copy; {{ currentYear }} {{ brandName }}. {{ copyright }}
            </p>
            <div class="mt-4 flex space-x-6 md:mt-0">
              <a
                v-for="legal in legalLinks"
                :key="legal.name"
                :href="legal.href"
                class="text-sm text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200"
                @click.prevent="handleLinkClick(legal)"
              >
                {{ legal.name }}
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>
  </footer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { Component } from 'vue'
import { IconGithub, IconEmail } from '@arco-design/web-vue/es/icon'

interface Link {
  name: string
  href: string
  external?: boolean
}

interface SocialLink {
  name: string
  url: string
  icon: Component
}

withDefaults(
  defineProps<{
    brandName?: string
    description?: string
    copyright?: string
    quickLinks?: Link[]
    supportLinks?: Link[]
    legalLinks?: Link[]
    socialLinks?: SocialLink[]
  }>(),
  {
    brandName: 'Paigram',
    copyright: 'All rights reserved.',
    quickLinks: () => [
      { name: '首页', href: '/' },
      { name: '控制台', href: '/dashboard' },
      { name: '个人资料', href: '/profile' },
      { name: '账号设置', href: '/settings' },
    ],
    supportLinks: () => [
      { name: '帮助文档', href: 'https://docs.paigram.com', external: true },
      { name: 'API 文档', href: '/api-docs' },
      { name: '联系我们', href: '/contact' },
      { name: '反馈建议', href: '/feedback' },
    ],
    legalLinks: () => [
      { name: '隐私政策', href: '/privacy' },
      { name: '服务条款', href: '/terms' },
      { name: '使用协议', href: '/agreement' },
    ],
    socialLinks: () => [
      { name: 'GitHub', url: 'https://github.com/paigram', icon: IconGithub },
      { name: 'Email', url: 'mailto:support@paigram.com', icon: IconEmail },
    ],
  }
)

const router = useRouter()

// 当前年份
const currentYear = computed(() => new Date().getFullYear())

// 处理链接点击
const handleLinkClick = (link: Link): void => {
  if (!link.external && link.href.startsWith('/')) {
    router.push(link.href)
  }
}
</script>

<style scoped></style>
