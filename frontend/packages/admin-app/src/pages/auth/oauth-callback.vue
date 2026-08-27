<template>
  <div class="relative flex min-h-screen items-center justify-center overflow-hidden bg-slate-950 px-4 py-10">
    <div
      class="absolute inset-0 bg-[radial-gradient(circle_at_top,_rgba(56,189,248,0.22),_transparent_42%),linear-gradient(135deg,_#020617,_#111827_55%,_#0f172a)]"
    ></div>
    <div class="absolute top-10 left-8 h-40 w-40 rounded-full bg-cyan-400/10 blur-3xl"></div>
    <div class="absolute right-6 bottom-8 h-48 w-48 rounded-full bg-sky-500/10 blur-3xl"></div>

    <div
      class="relative w-full max-w-md rounded-3xl border border-white/10 bg-white/95 p-8 text-center shadow-2xl shadow-sky-950/30 backdrop-blur"
    >
      <div v-if="loading" class="space-y-4">
        <a-spin size="large" />
        <p class="text-xs font-semibold tracking-[0.35em] text-sky-600 uppercase">Paigram Console</p>
        <h2 class="text-2xl font-semibold text-slate-900">正在处理授权</h2>
        <p class="text-sm text-slate-500">请稍候，我们正在完成 {{ providerName }} 登录。</p>
      </div>

      <div v-else-if="error" class="space-y-4">
        <div class="flex justify-center">
          <icon-close-circle-fill class="text-6xl text-red-500" />
        </div>
        <p class="text-xs font-semibold tracking-[0.35em] text-red-500 uppercase">OAuth Error</p>
        <h2 class="text-2xl font-semibold text-slate-900">授权失败</h2>
        <div class="rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-600">
          {{ error }}
        </div>
        <div class="flex flex-col gap-3 pt-2">
          <a-button type="primary" long @click="retry">
            <template #icon>
              <icon-refresh />
            </template>
            重试
          </a-button>
          <a-button long @click="backToLogin">返回登录页</a-button>
        </div>
      </div>

      <div v-else class="space-y-4">
        <div class="flex justify-center">
          <icon-check-circle-fill class="text-6xl text-emerald-500" />
        </div>
        <p class="text-xs font-semibold tracking-[0.35em] text-emerald-600 uppercase">Authorized</p>
        <h2 class="text-2xl font-semibold text-slate-900">授权成功</h2>
        <p class="text-sm text-slate-500">正在根据你的权限跳转到可访问的控制台入口。</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { IconCheckCircleFill, IconCloseCircleFill, IconRefresh } from '@arco-design/web-vue/es/icon'
import { useAuthStore } from '@/stores/auth'
import { isSafeInternalPath } from '@/utils/navigation'
import type { OAuthCallbackRequest } from '@paigram/shared-components'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const loading = ref(true)
const error = ref('')

const providerName = computed(() => {
  const provider = route.params.provider as string
  const nameMap: Record<string, string> = {
    google: 'Google',
    github: 'GitHub',
  }

  return nameMap[provider] || provider
})

const handleCallback = async (): Promise<void> => {
  const provider = route.params.provider as string

  if (!provider) {
    error.value = '缺少 OAuth 提供商参数'
    loading.value = false
    return
  }

  const { code, state, error: errorParam, error_description: errorDescription } = route.query

  if (errorParam) {
    error.value = (errorDescription as string) || `授权失败：${errorParam}`
    loading.value = false
    Message.error(error.value)
    return
  }

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

    const fallbackPath = await authStore.handleOAuthCallback(provider, callbackData)
    const returnKey = `oauth-admin-return:${provider}`
    const redirect = sessionStorage.getItem(returnKey) || ''
    sessionStorage.removeItem(returnKey)

    setTimeout(() => {
      router.replace(isSafeInternalPath(redirect) ? redirect : fallbackPath)
    }, 1000)
  } catch (err) {
    console.error('OAuth callback error:', err)
    const errorObj = err as { error?: string; message?: string; response?: { data?: { error?: string } } }
    error.value =
      errorObj.response?.data?.error || errorObj.error || errorObj.message || `${providerName.value} 授权失败，请重试`
    loading.value = false
  }
}

const retry = (): void => {
  error.value = ''
  loading.value = true
  void handleCallback()
}

const backToLogin = (): void => {
  router.push('/login')
}

onMounted(() => {
  void handleCallback()
})
</script>
