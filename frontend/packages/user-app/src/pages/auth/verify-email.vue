<template>
  <div class="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-900">
    <div class="w-full max-w-md rounded-lg bg-white p-8 text-center shadow-lg dark:bg-gray-800">
      <!-- 加载状态 -->
      <div v-if="loading" class="space-y-4">
        <a-spin size="large" />
        <h2 class="text-xl font-semibold text-gray-800 dark:text-white">正在验证邮箱...</h2>
        <p class="text-sm text-gray-600 dark:text-gray-400">请稍候，我们正在验证您的邮箱地址</p>
      </div>

      <!-- 错误状态 -->
      <div v-else-if="error" class="space-y-4">
        <div class="flex justify-center">
          <icon-close-circle-fill class="text-6xl text-red-500" />
        </div>
        <h2 class="text-xl font-semibold text-gray-800 dark:text-white">验证失败</h2>
        <div class="rounded-lg bg-red-50 p-4 dark:bg-red-900/20">
          <p class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
        </div>
        <div class="flex flex-col gap-3 pt-4">
          <a-button type="primary" long @click="retry">
            <template #icon>
              <icon-refresh />
            </template>
            重试验证
          </a-button>
          <a-button long @click="backToLogin">返回登录页</a-button>
        </div>
      </div>

      <!-- 成功状态 -->
      <div v-else class="space-y-4">
        <div class="flex justify-center">
          <icon-check-circle-fill class="text-6xl text-green-500" />
        </div>
        <h2 class="text-xl font-semibold text-gray-800 dark:text-white">邮箱验证成功</h2>
        <p class="text-sm text-gray-600 dark:text-gray-400">您的邮箱已成功验证，即将跳转到登录页...</p>
        <div class="pt-4">
          <a-button type="primary" long @click="goToLogin">立即登录</a-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { IconCloseCircleFill, IconCheckCircleFill, IconRefresh } from '@arco-design/web-vue/es/icon'
import { authApi } from '@/api'

const route = useRoute()
const router = useRouter()

const loading = ref(true)
const error = ref('')

// 处理邮箱验证
const handleVerifyEmail = async (): Promise<void> => {
  // 获取 URL 参数
  const { email, token } = route.query

  // 验证必需参数
  if (!email || typeof email !== 'string') {
    error.value = '验证失败：缺少邮箱参数'
    loading.value = false
    Message.error(error.value)
    return
  }

  if (!token || typeof token !== 'string') {
    error.value = '验证失败：缺少验证令牌'
    loading.value = false
    Message.error(error.value)
    return
  }

  try {
    // 调用验证 API
    const response = await authApi.verifyEmail({
      email,
      token,
    })

    // 验证成功
    Message.success(response.data?.message || '邮箱验证成功')
    loading.value = false

    // 延迟 2 秒后自动跳转到登录页
    setTimeout(() => {
      router.replace('/login')
    }, 2000)
  } catch (err) {
    console.error('Email verification error:', err)
    const errorObj = err as { error?: string; message?: string; response?: { data?: { error?: string } } }

    // 尝试从不同来源获取错误信息
    error.value = errorObj.response?.data?.error || errorObj.error || errorObj.message || '邮箱验证失败，请重试'

    loading.value = false
    Message.error(error.value)
  }
}

// 重试验证
const retry = (): void => {
  error.value = ''
  loading.value = true
  handleVerifyEmail()
}

// 返回登录页
const backToLogin = (): void => {
  router.push('/login')
}

// 立即跳转到登录页
const goToLogin = (): void => {
  router.replace('/login')
}

onMounted(() => {
  handleVerifyEmail()
})
</script>
