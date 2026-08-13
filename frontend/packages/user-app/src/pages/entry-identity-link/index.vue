<template>
  <main class="flex min-h-screen items-center justify-center bg-slate-50 px-4 py-10 dark:bg-slate-950">
    <a-card class="w-full max-w-2xl" :bordered="true">
      <h1 class="mb-2 text-2xl font-semibold text-slate-900 dark:text-slate-100">确认 Bot 身份关联</h1>
      <p class="mb-6 text-sm leading-6 text-slate-600 dark:text-slate-300">
        请核对发起关联的 Bot 和已脱敏的外部身份。确认后，该 Bot 可代表此身份访问你明确授权的平台账号。
      </p>

      <p class="sr-only" role="status" aria-live="polite">{{ liveMessage }}</p>

      <a-spin
        v-if="state === 'loading'"
        class="flex min-h-48 w-full items-center justify-center"
        tip="正在验证关联请求"
      />

      <template v-else-if="state === 'ready' && preview">
        <a-alert class="mb-6" type="warning" title="仅在你刚刚从该 Bot 发起操作时确认">
          如果你不认识这个 Bot 或没有发起关联，请取消请求。我们不会向 Bot 返回你的 Account Center 用户 ID。
        </a-alert>
        <a-descriptions :column="1" bordered layout="horizontal">
          <a-descriptions-item label="Bot">{{ preview.bot_display_name }}（{{ preview.bot_id }}）</a-descriptions-item>
          <a-descriptions-item label="身份命名空间">{{ preview.issuer }}</a-descriptions-item>
          <a-descriptions-item label="外部身份">{{ preview.masked_subject }}</a-descriptions-item>
          <a-descriptions-item label="请求有效期">{{ formatDate(preview.expires_at) }}</a-descriptions-item>
        </a-descriptions>
        <div class="mt-6 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <a-button size="large" :loading="submitting" @click="cancelLink">取消关联</a-button>
          <a-button type="primary" size="large" :loading="submitting" @click="approveLink">确认关联</a-button>
        </div>
      </template>

      <a-result
        v-else-if="state === 'approved'"
        status="success"
        title="身份关联成功"
        subtitle="你现在可以关闭此页面并返回 Bot。"
      >
        <template #extra>
          <a-button type="primary" href="/apps">查看应用授权</a-button>
        </template>
      </a-result>

      <a-result
        v-else-if="state === 'cancelled'"
        status="info"
        title="关联请求已取消"
        subtitle="该一次性请求不能再次使用。"
      />

      <a-result v-else role="alert" status="error" title="无法确认关联" :subtitle="errorMessage">
        <template #extra>
          <a-button href="/">返回首页</a-button>
        </template>
      </a-result>
    </a-card>
  </main>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@paigram/shared-components'
import type { EntryIdentityChallenge } from '@paigram/shared-components'
import { platformAccountsApi } from '@/api'
import {
  captureEntryIdentityChallenge,
  clearEntryIdentityChallenge,
  isTerminalEntryIdentityChallengeError,
} from '@/features/entry-identity-link/challenge'

type PageState = 'loading' | 'ready' | 'approved' | 'cancelled' | 'error'

const router = useRouter()
const userStore = useUserStore()
const challenge = captureEntryIdentityChallenge()
const state = ref<PageState>('loading')
const preview = ref<EntryIdentityChallenge | null>(null)
const submitting = ref(false)
const errorMessage = ref('此请求不存在、已过期或已被使用。请返回 Bot 重新发起。')
const liveMessage = ref('正在验证关联请求')

onMounted(async () => {
  if (!challenge) {
    showError('关联请求缺失或格式无效。请返回 Bot 重新发起。')
    return
  }
  if (!userStore.isLogin) {
    liveMessage.value = '需要登录后继续确认'
    await router.replace({ path: '/login', query: { redirect: '/entry-identity-link' } })
    return
  }
  try {
    const response = await platformAccountsApi.previewEntryIdentityLink(challenge)
    preview.value = response.data
    state.value = 'ready'
    liveMessage.value = '关联请求已验证，请核对信息'
  } catch (error: unknown) {
    if (isTerminalEntryIdentityChallengeError(error)) clearEntryIdentityChallenge()
    showError(readErrorMessage(error))
  }
})

async function approveLink(): Promise<void> {
  if (!challenge || submitting.value) return
  submitting.value = true
  try {
    await platformAccountsApi.approveEntryIdentityLink(challenge)
    clearEntryIdentityChallenge()
    state.value = 'approved'
    liveMessage.value = '身份关联成功'
  } catch (error: unknown) {
    if (isTerminalEntryIdentityChallengeError(error)) clearEntryIdentityChallenge()
    showError(readErrorMessage(error))
  } finally {
    submitting.value = false
  }
}

async function cancelLink(): Promise<void> {
  if (!challenge || submitting.value) return
  submitting.value = true
  try {
    await platformAccountsApi.cancelEntryIdentityLink(challenge)
    clearEntryIdentityChallenge()
    state.value = 'cancelled'
    liveMessage.value = '关联请求已取消'
  } catch (error: unknown) {
    if (isTerminalEntryIdentityChallengeError(error)) clearEntryIdentityChallenge()
    showError(readErrorMessage(error))
  } finally {
    submitting.value = false
  }
}

function showError(message: string): void {
  errorMessage.value = message
  state.value = 'error'
  liveMessage.value = message
}

function readErrorMessage(error: unknown): string {
  const message = (error as { message?: unknown })?.message
  return typeof message === 'string' && message.length > 0
    ? message
    : '此请求不存在、已过期或已被使用。请返回 Bot 重新发起。'
}

function formatDate(value: string): string {
  return new Date(value).toLocaleString('zh-CN')
}
</script>
