<template>
  <div class="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-900">
    <div class="w-full max-w-md rounded-lg bg-white p-8 text-center shadow-lg dark:bg-gray-800">
      <!-- 加载状态 -->
      <div v-if="loading" class="space-y-4">
        <a-spin size="large" />
        <h2 class="text-xl font-semibold text-gray-800 dark:text-white">正在处理授权...</h2>
        <p class="text-sm text-gray-600 dark:text-gray-400">请稍候，我们正在完成 {{ providerName }} 登录</p>
      </div>

      <!-- 错误状态 -->
      <div v-else-if="error" class="space-y-4">
        <div class="flex justify-center">
          <icon-close-circle-fill class="text-6xl text-red-500" />
        </div>
        <h2 class="text-xl font-semibold text-gray-800 dark:text-white">授权失败</h2>
        <div class="rounded-lg bg-red-50 p-4 dark:bg-red-900/20">
          <p class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
        </div>
        <div class="flex flex-col gap-3 pt-4">
          <a-button type="primary" long @click="retry">
            <template #icon>
              <icon-refresh />
            </template>
            重试
          </a-button>
          <a-button long @click="backToLogin">返回登录页</a-button>
        </div>
      </div>

      <!-- 成功状态（短暂显示） -->
      <div v-else class="space-y-4">
        <div class="flex justify-center">
          <icon-check-circle-fill class="text-6xl text-green-500" />
        </div>
        <h2 class="text-xl font-semibold text-gray-800 dark:text-white">授权成功</h2>
        <p class="text-sm text-gray-600 dark:text-gray-400">即将跳转...</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { IconCloseCircleFill, IconCheckCircleFill, IconRefresh } from '@arco-design/web-vue/es/icon'
import { useAuthStore } from '@/stores/auth'
import type { OAuthCallbackRequest } from '@paigram/shared-components'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const loading = ref(true)
const error = ref('')

// 获取 provider 名称用于显示
const providerName = computed(() => {
  const provider = route.params.provider as string
  const nameMap: Record<string, string> = {
    google: 'Google',
    github: 'GitHub',
    telegram: 'Telegram',
  }
  return nameMap[provider] || provider
})

// 处理 OAuth 回调
const handleCallback = async (): Promise<void> => {
  const provider = route.params.provider as string

  // 验证 provider
  if (!provider) {
    error.value = '缺少 OAuth 提供商参数'
    loading.value = false
    return
  }

  // 获取 URL 参数
  const { code, state, error: errorParam, error_description: errorDescription } = route.query

  // 处理 OAuth 错误
  if (errorParam) {
    error.value = (errorDescription as string) || `授权失败：${errorParam}`
    loading.value = false
    Message.error(error.value)
    return
  }

  // 验证必需参数
  if (!code) {
    error.value = '授权失败：缺少授权码'
    loading.value = false
    Message.error(error.value)
    return
  }

  if (!state) {
    error.value = '授权失败：缺少状态参数'
    loading.value = false
    Message.error(error.value)
    return
  }

  try {
    const callbackData: OAuthCallbackRequest = {
      code: code as string,
      state: state as string,
    }

    // 调用 auth store 处理回调
    await authStore.handleOAuthCallback(provider, callbackData)

    // 成功后跳转
    const redirectTo = (route.query.redirect_to as string) || '/dashboard'

    // 短暂延迟以显示成功状态
    setTimeout(() => {
      router.replace(redirectTo)
    }, 1000)
  } catch (err) {
    console.error('OAuth callback error:', err)
    const errorObj = err as { error?: string; message?: string; response?: { data?: { error?: string } } }

    // 尝试从不同来源获取错误信息
    error.value =
      errorObj.response?.data?.error || errorObj.error || errorObj.message || `${providerName.value} 授权失败，请重试`

    loading.value = false
  }
}

// 重试
const retry = (): void => {
  error.value = ''
  loading.value = true
  void handleCallback()
}

// 返回登录页
const backToLogin = (): void => {
  router.push('/login')
}

onMounted(() => {
  void handleCallback()
})
</script>
