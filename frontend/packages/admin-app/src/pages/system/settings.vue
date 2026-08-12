<template>
  <div class="space-y-4 p-6">
    <div>
      <h1 class="text-2xl font-semibold">系统设置</h1>
      <p class="mt-1 text-gray-500">编辑服务端已注册的设置域；保存前会验证 JSON 对象格式。</p>
    </div>
    <a-card>
      <a-tabs v-model:active-key="activeDomain" @change="loadDomain(String($event))">
        <a-tab-pane v-for="domain in domains" :key="domain.key" :title="domain.label" />
      </a-tabs>
      <a-spin :loading="loading" class="w-full">
        <a-alert class="mb-3" type="info">当前版本：{{ version }}；敏感值不会由接口回显。</a-alert>
        <a-textarea v-model="settingsJSON" :auto-size="{ minRows: 14, maxRows: 28 }" />
        <a-button
          v-if="userStore.hasPermission('system:update')"
          class="mt-4"
          type="primary"
          :loading="saving"
          @click="saveDomain"
        >
          保存设置
        </a-button>
      </a-spin>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { useUserStore } from '@paigram/shared-components'
import { adminSystemApi } from '@/api'

const userStore = useUserStore()
const domains = [
  { key: 'site', label: '站点' },
  { key: 'registration', label: '注册' },
  { key: 'email', label: '邮件' },
  { key: 'auth-controls', label: '认证控制' },
]
const activeDomain = ref('site')
const settingsJSON = ref('{}')
const version = ref(0)
const loading = ref(false)
const saving = ref(false)

const loadDomain = async (domain: string): Promise<void> => {
  loading.value = true
  try {
    const response = await adminSystemApi.getSettings(domain)
    version.value = response.data.version
    settingsJSON.value = JSON.stringify(response.data.settings, null, 2)
  } finally {
    loading.value = false
  }
}

const saveDomain = async (): Promise<void> => {
  let settings: Record<string, unknown>
  try {
    const value: unknown = JSON.parse(settingsJSON.value)
    if (!value || Array.isArray(value) || typeof value !== 'object') throw new Error('invalid')
    settings = value as Record<string, unknown>
  } catch (_error) {
    Message.error('设置内容必须是有效的 JSON 对象')
    return
  }
  saving.value = true
  try {
    const response = await adminSystemApi.patchSettings(activeDomain.value, settings)
    version.value = response.data.version
    settingsJSON.value = JSON.stringify(response.data.settings, null, 2)
    Message.success('系统设置已保存')
  } finally {
    saving.value = false
  }
}

onMounted(() => void loadDomain(activeDomain.value))
</script>
