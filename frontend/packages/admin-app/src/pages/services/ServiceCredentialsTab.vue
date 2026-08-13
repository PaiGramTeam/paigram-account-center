<template>
  <div class="mb-4 flex justify-end">
    <a-button v-if="hasPermission('bot:create')" type="primary" @click="openCreate"> 新建服务凭据 </a-button>
  </div>
  <a-alert class="mb-4" type="warning">
    客户端密钥只在创建或轮换后显示一次。请立即保存到对应服务的密钥管理系统。
  </a-alert>
  <a-table row-key="client_id" :columns="columns" :data="credentials" :loading="loading" :pagination="false">
    <template #identity="{ record }">
      <div class="font-medium">{{ record.display_name }}</div>
      <div class="text-xs text-gray-500">{{ record.client_id }}</div>
    </template>
    <template #bot="{ record }">
      <code>{{ record.bot_id }}</code>
      <div class="mt-1 text-xs break-all text-gray-500">{{ record.entry_issuer }}</div>
    </template>
    <template #scopes="{ record }">
      <a-space wrap size="mini">
        <a-tag v-for="scope in record.scopes || []" :key="scope">{{ scope }}</a-tag>
      </a-space>
      <span v-if="!record.scopes?.length" class="text-gray-400">无</span>
    </template>
    <template #status="{ record }">
      <a-tag :color="record.status === 'active' ? 'green' : 'gray'">{{ record.status }}</a-tag>
    </template>
    <template #actions="{ record }">
      <a-popconfirm
        v-if="hasPermission('bot:update')"
        content="轮换后旧密钥立即失效，确认继续？"
        type="warning"
        @ok="rotate(record.client_id)"
      >
        <a-button type="text" size="small" :loading="rotatingClientId === record.client_id"> 轮换密钥 </a-button>
      </a-popconfirm>
    </template>
  </a-table>
  <a-empty v-if="!loading && credentials.length === 0" description="暂无服务凭据" />

  <a-modal
    v-model:visible="createVisible"
    title="新建服务凭据"
    :mask-closable="false"
    :on-before-ok="create"
    @close="resetForm"
  >
    <a-form :model="form" layout="vertical">
      <a-form-item label="客户端 ID" required>
        <a-input v-model="form.client_id" placeholder="telegram-service" />
      </a-form-item>
      <a-form-item label="逻辑 Bot ID" required>
        <a-input v-model="form.bot_id" placeholder="paigram" />
      </a-form-item>
      <a-form-item label="外部身份命名空间">
        <a-input v-model="form.entry_issuer" placeholder="留空时使用 urn:paigram:entry:<Bot ID>" />
      </a-form-item>
      <a-form-item label="显示名称" required><a-input v-model="form.display_name" /></a-form-item>
      <a-form-item label="描述"><a-textarea v-model="form.description" /></a-form-item>
      <a-form-item label="OAuth 受众" required>
        <a-textarea v-model="form.audiencesText" :auto-size="{ minRows: 2, maxRows: 5 }" placeholder="每行一个受众" />
      </a-form-item>
      <a-form-item label="OAuth Scope" required>
        <a-textarea v-model="form.scopesText" :auto-size="{ minRows: 3, maxRows: 7 }" placeholder="每行一个 scope" />
      </a-form-item>
    </a-form>
  </a-modal>

  <a-modal
    v-model:visible="secretVisible"
    title="请立即保存客户端密钥"
    :hide-cancel="true"
    :mask-closable="false"
    ok-text="我已保存"
    @close="clearSecret"
  >
    <a-alert class="mb-4" type="warning">关闭后无法再次查看此密钥，只能重新轮换。</a-alert>
    <a-descriptions :column="1" bordered>
      <a-descriptions-item label="Client ID"
        ><code>{{ oneTimeClientId }}</code></a-descriptions-item
      >
      <a-descriptions-item label="Client Secret">
        <div class="flex items-start gap-2">
          <code class="break-all">{{ oneTimeSecret }}</code>
          <a-button size="mini" @click="copySecret">复制</a-button>
        </div>
      </a-descriptions-item>
    </a-descriptions>
  </a-modal>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { useUserStore, type ServiceCredential } from '@paigram/shared-components'
import { adminServicesApi } from '@/api'

const userStore = useUserStore()
const hasPermission = (permission: string): boolean => userStore.hasPermission(permission)
const columns = [
  { title: '凭据', slotName: 'identity', width: 220 },
  { title: '逻辑 Bot', slotName: 'bot', width: 160 },
  { title: 'Scope', slotName: 'scopes' },
  { title: '状态', slotName: 'status', width: 100 },
  { title: '操作', slotName: 'actions', width: 110 },
]
const credentials = ref<ServiceCredential[]>([])
const loading = ref(false)
const rotatingClientId = ref('')
const createVisible = ref(false)
const secretVisible = ref(false)
const oneTimeClientId = ref('')
const oneTimeSecret = ref('')
const form = reactive({
  client_id: '',
  bot_id: '',
  entry_issuer: '',
  display_name: '',
  description: '',
  audiencesText: 'account-center',
  scopesText: 'bot.access.read\nbot.access.issue_ticket\nbot.access.link_identity',
})

const splitLines = (value: string): string[] => [
  ...new Set(
    value
      .split(/[\n,]/)
      .map((item) => item.trim())
      .filter(Boolean)
  ),
]

const load = async (): Promise<void> => {
  loading.value = true
  try {
    credentials.value = (await adminServicesApi.listServiceCredentials()).data ?? []
  } finally {
    loading.value = false
  }
}

const openCreate = (): void => {
  resetForm()
  createVisible.value = true
}

const create = async (): Promise<boolean> => {
  if (!form.client_id.trim() || !form.bot_id.trim() || !form.display_name.trim()) {
    Message.warning('请填写客户端 ID、逻辑 Bot ID 和显示名称')
    return false
  }
  const audiences = splitLines(form.audiencesText)
  const scopes = splitLines(form.scopesText)
  if (audiences.length === 0 || scopes.length === 0) {
    Message.warning('至少需要一个受众和一个 Scope')
    return false
  }
  try {
    const result = (
      await adminServicesApi.createServiceCredential({
        client_id: form.client_id.trim(),
        bot_id: form.bot_id.trim(),
        entry_issuer: form.entry_issuer.trim() || undefined,
        display_name: form.display_name.trim(),
        description: form.description.trim(),
        audiences,
        scopes,
      })
    ).data
    revealSecret(result.client_id, result.client_secret)
    await load()
    return true
  } catch (_error) {
    return false
  }
}

const rotate = async (clientId: string): Promise<void> => {
  rotatingClientId.value = clientId
  try {
    const result = (await adminServicesApi.rotateServiceCredential(clientId)).data
    revealSecret(result.client_id, result.client_secret)
    await load()
  } finally {
    rotatingClientId.value = ''
  }
}

const revealSecret = (clientId: string, secret: string): void => {
  oneTimeClientId.value = clientId
  oneTimeSecret.value = secret
  secretVisible.value = true
}

const clearSecret = (): void => {
  oneTimeClientId.value = ''
  oneTimeSecret.value = ''
}

const copySecret = async (): Promise<void> => {
  try {
    await navigator.clipboard.writeText(oneTimeSecret.value)
    Message.success('密钥已复制')
  } catch (_error) {
    Message.error('无法访问剪贴板，请手动复制')
  }
}

const resetForm = (): void => {
  Object.assign(form, {
    client_id: '',
    bot_id: '',
    entry_issuer: '',
    display_name: '',
    description: '',
    audiencesText: 'account-center',
    scopesText: 'bot.access.read\nbot.access.issue_ticket\nbot.access.link_identity',
  })
}

onMounted(() => void load())
</script>
