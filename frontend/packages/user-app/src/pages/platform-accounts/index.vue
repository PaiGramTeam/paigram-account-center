<template>
  <div class="space-y-4 p-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-semibold">平台账号</h1>
        <p class="mt-1 text-gray-500">集中管理平台凭据、游戏角色和应用授权。</p>
      </div>
      <a-button type="primary" @click="openCreate">绑定平台账号</a-button>
    </div>

    <a-card>
      <a-table :columns="columns" :data="bindings" row-key="id" :loading="loading" :pagination="false">
        <template #status="{ record }">
          <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
        </template>
        <template #account="{ record }">{{ record.external_account_key || '等待平台确认' }}</template>
        <template #actions="{ record }"><a-button type="text" @click="openDetail(record)">管理</a-button></template>
      </a-table>
      <a-pagination
        v-if="pagination.total > pagination.page_size"
        v-model:current="pagination.page"
        class="mt-4 justify-end"
        :page-size="pagination.page_size"
        :total="pagination.total"
        @change="loadBindings"
      />
    </a-card>
  </div>

  <a-modal v-model:visible="createVisible" title="绑定平台账号" :on-before-ok="createBinding" @close="resetCreateForm">
    <a-form :model="createForm" layout="vertical">
      <a-form-item field="platform" label="平台" required>
        <a-select v-model="createForm.platform" placeholder="请选择平台">
          <a-option v-for="platform in platforms" :key="platform.platform" :value="platform.platform">{{
            platform.display_name
          }}</a-option>
        </a-select>
      </a-form-item>
      <a-form-item field="displayName" label="显示名称" required>
        <a-input v-model="createForm.displayName" placeholder="例如：我的主账号" />
      </a-form-item>
      <a-form-item field="credentialJSON" label="凭据 JSON" required>
        <a-textarea
          v-model="createForm.credentialJSON"
          :auto-size="{ minRows: 6, maxRows: 12 }"
          placeholder='例如：{"cookie_token":"..."}'
        />
        <template #extra>凭据仅用于本次绑定，不会保存在浏览器或 Account Center。</template>
      </a-form-item>
    </a-form>
  </a-modal>

  <PlatformAccountDetail
    v-model:visible="detailVisible"
    :binding="selectedBinding"
    @changed="loadBindings"
    @deleted="loadBindings"
  />
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { platformAccountsApi } from '@/api'
import PlatformAccountDetail from '@/features/platform-accounts/PlatformAccountDetail.vue'
import type { Pagination, PlatformBinding, PlatformDefinition } from '@paigram/shared-components'

const columns = [
  { title: '名称', dataIndex: 'display_name' },
  { title: '平台', dataIndex: 'platform' },
  { title: '外部账号', slotName: 'account' },
  { title: '状态', slotName: 'status' },
  { title: '操作', slotName: 'actions', width: 100 },
]
const loading = ref(false)
const createVisible = ref(false)
const detailVisible = ref(false)
const selectedBinding = ref<PlatformBinding | null>(null)
const bindings = ref<PlatformBinding[]>([])
const platforms = ref<PlatformDefinition[]>([])
const pagination = reactive<Pagination>({ page: 1, page_size: 20, total: 0, total_pages: 0 })
const createForm = reactive({ platform: '', displayName: '', credentialJSON: '' })

const loadBindings = async (): Promise<void> => {
  loading.value = true
  try {
    const response = await platformAccountsApi.list({ page: pagination.page, page_size: pagination.page_size })
    bindings.value = response.data.items ?? []
    Object.assign(pagination, response.data.pagination)
  } finally {
    loading.value = false
  }
}

const openCreate = async (): Promise<void> => {
  if (platforms.value.length === 0) {
    platforms.value = (await platformAccountsApi.listPlatforms()).data ?? []
  }
  createVisible.value = true
}

const createBinding = async (): Promise<boolean> => {
  if (!createForm.platform || !createForm.displayName.trim()) {
    Message.error('请选择平台并填写显示名称')
    return false
  }
  let credential: Record<string, unknown>
  try {
    const parsed: unknown = JSON.parse(createForm.credentialJSON)
    if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error('invalid')
    credential = parsed as Record<string, unknown>
  } catch (_error) {
    Message.error('凭据必须是有效的 JSON 对象')
    return false
  }
  try {
    await platformAccountsApi.create({
      platform: createForm.platform,
      display_name: createForm.displayName.trim(),
      credential_payload: credential,
    })
    Message.success('平台账号绑定成功')
    resetCreateForm()
    await loadBindings()
    return true
  } catch (_error) {
    return false
  }
}

const resetCreateForm = (): void => {
  createForm.platform = ''
  createForm.displayName = ''
  createForm.credentialJSON = ''
}
const openDetail = (binding: PlatformBinding): void => {
  selectedBinding.value = binding
  detailVisible.value = true
}
const statusColor = (status: string): string =>
  ({ active: 'green', credential_invalid: 'red', refresh_required: 'orange' })[status] ?? 'gray'

onMounted(() => void loadBindings())
</script>
