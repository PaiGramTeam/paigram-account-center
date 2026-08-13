<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-semibold">平台账号</h1>
      <p class="mt-1 text-gray-500">查看用户的平台绑定、角色、运行状态和消费者授权。</p>
    </div>

    <a-card :bordered="false">
      <a-table :columns="columns" :data="bindings" row-key="id" :loading="loading" :pagination="false">
        <template #owner="{ record }">#{{ record.owner_user_id }}</template>
        <template #account="{ record }">{{ record.external_account_key || '等待平台确认' }}</template>
        <template #status="{ record }">
          <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
        </template>
        <template #actions="{ record }">
          <a-button type="text" @click="openDetail(record)">查看</a-button>
        </template>
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

  <a-drawer :visible="detailVisible" :width="760" title="平台账号详情" @cancel="detailVisible = false">
    <a-spin :loading="detailLoading" class="w-full">
      <a-descriptions v-if="selectedBinding" :column="2" bordered>
        <a-descriptions-item label="绑定 ID">{{ selectedBinding.id }}</a-descriptions-item>
        <a-descriptions-item label="用户 ID">{{ selectedBinding.owner_user_id }}</a-descriptions-item>
        <a-descriptions-item label="名称">{{ selectedBinding.display_name }}</a-descriptions-item>
        <a-descriptions-item label="平台">{{ selectedBinding.platform }}</a-descriptions-item>
        <a-descriptions-item label="状态">
          <a-tag :color="statusColor(selectedBinding.status)">{{ selectedBinding.status }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="运行状态">{{ runtime?.status || '-' }}</a-descriptions-item>
      </a-descriptions>

      <a-space v-if="selectedBinding" class="my-4" wrap>
        <a-button
          v-if="hasPermission('platform_account:update')"
          type="primary"
          :loading="refreshing"
          @click="refreshBinding"
        >
          刷新凭据
        </a-button>
        <a-button v-if="hasPermission('platform_account:update')" @click="credentialVisible = true">
          更换凭据
        </a-button>
        <a-popconfirm
          v-if="hasPermission('platform_account:delete')"
          content="删除后平台凭据也会被清理，确认继续？"
          type="warning"
          @ok="removeBinding"
        >
          <a-button status="danger" :loading="removing">删除账号</a-button>
        </a-popconfirm>
      </a-space>

      <a-tabs>
        <a-tab-pane key="profiles" title="游戏角色">
          <a-list :data="profiles" :bordered="false">
            <template #item="{ item }">
              <a-list-item>
                <a-list-item-meta
                  :title="item.nickname || item.player_uid"
                  :description="`${item.game_biz} · ${item.region} · ${item.player_uid}`"
                />
                <template #actions><a-tag v-if="item.is_primary" color="green">主角色</a-tag></template>
              </a-list-item>
            </template>
          </a-list>
          <a-empty v-if="profiles.length === 0" description="暂无角色" />
        </a-tab-pane>
        <a-tab-pane key="grants" title="应用授权">
          <div v-if="hasPermission('platform_account:update')" class="mb-4 flex gap-2">
            <a-input v-model="newConsumer" placeholder="输入已注册的消费者 ID" @press-enter="enableConsumer" />
            <a-button type="primary" :loading="grantSaving" @click="enableConsumer">授权应用</a-button>
          </div>
          <div v-for="grant in grants" :key="grant.consumer" class="mb-3 rounded border border-gray-200 p-4">
            <div class="flex items-center justify-between">
              <div>
                <div class="font-medium">{{ grant.consumer }}</div>
                <div class="mt-1 text-xs text-gray-500">{{ grant.actions?.join('、') || '未授权' }}</div>
              </div>
              <a-switch
                :model-value="grant.status === 'active'"
                :disabled="!hasPermission('platform_account:update')"
                @change="(value) => changeGrant(grant.consumer, Boolean(value))"
              />
            </div>
          </div>
          <a-empty v-if="grants.length === 0" description="暂无消费者授权" />
        </a-tab-pane>
        <a-tab-pane v-if="selectedBinding" key="operations" title="协调恢复">
          <OperationRecoveryPanel
            :binding-id="selectedBinding.id"
            :can-requeue="hasPermission('platform_account:update')"
          />
        </a-tab-pane>
      </a-tabs>
    </a-spin>
  </a-drawer>

  <a-modal
    v-model:visible="credentialVisible"
    title="更换平台凭据"
    :on-before-ok="updateCredential"
    @close="credentialJSON = ''"
  >
    <a-alert type="warning" class="mb-3">凭据仅用于本次提交，页面不会保存输入内容。</a-alert>
    <a-textarea
      v-model="credentialJSON"
      :auto-size="{ minRows: 6, maxRows: 12 }"
      placeholder='例如：{"cookie_token":"..."}'
    />
  </a-modal>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { adminPlatformAccountsApi } from '@/api'
import OperationRecoveryPanel from './OperationRecoveryPanel.vue'
import {
  useUserStore,
  type AdminPlatformBinding,
  type ConsumerGrant,
  type Pagination,
  type PlatformProfile,
  type PlatformRuntimeSummary,
} from '@paigram/shared-components'

const userStore = useUserStore()
const columns = [
  { title: '名称', dataIndex: 'display_name' },
  { title: '用户', slotName: 'owner', width: 100 },
  { title: '平台', dataIndex: 'platform' },
  { title: '外部账号', slotName: 'account' },
  { title: '状态', slotName: 'status', width: 150 },
  { title: '操作', slotName: 'actions', width: 90 },
]
const bindings = ref<AdminPlatformBinding[]>([])
const selectedBinding = ref<AdminPlatformBinding | null>(null)
const profiles = ref<PlatformProfile[]>([])
const grants = ref<ConsumerGrant[]>([])
const runtime = ref<PlatformRuntimeSummary | null>(null)
const loading = ref(false)
const detailLoading = ref(false)
const refreshing = ref(false)
const removing = ref(false)
const detailVisible = ref(false)
const credentialVisible = ref(false)
const credentialJSON = ref('')
const newConsumer = ref('')
const grantSaving = ref(false)
const pagination = reactive<Pagination>({ page: 1, page_size: 20, total: 0, total_pages: 0 })

const hasPermission = (permission: string): boolean => userStore.hasPermission(permission)

const loadBindings = async (): Promise<void> => {
  loading.value = true
  try {
    const response = await adminPlatformAccountsApi.list({
      page: pagination.page,
      page_size: pagination.page_size,
    })
    bindings.value = response.data.items ?? []
    Object.assign(pagination, response.data.pagination)
  } finally {
    loading.value = false
  }
}

const loadDetail = async (): Promise<void> => {
  if (!selectedBinding.value) return
  detailLoading.value = true
  try {
    const bindingId = selectedBinding.value.id
    const [profileResponse, grantResponse, runtimeResponse] = await Promise.all([
      adminPlatformAccountsApi.listProfiles(bindingId),
      adminPlatformAccountsApi.listGrants(bindingId),
      adminPlatformAccountsApi.getRuntimeSummary(bindingId),
    ])
    profiles.value = profileResponse.data.items ?? []
    grants.value = grantResponse.data.items ?? []
    runtime.value = runtimeResponse.data
  } catch (_error) {
    Message.error('加载平台账号详情失败')
  } finally {
    detailLoading.value = false
  }
}

const openDetail = (binding: AdminPlatformBinding): void => {
  selectedBinding.value = binding
  detailVisible.value = true
  void loadDetail()
}

const refreshBinding = async (): Promise<void> => {
  if (!selectedBinding.value) return
  refreshing.value = true
  try {
    runtime.value = (await adminPlatformAccountsApi.refresh(selectedBinding.value.id)).data
    Message.success('平台账号已刷新')
    await Promise.all([loadBindings(), loadDetail()])
  } finally {
    refreshing.value = false
  }
}

const parseCredential = (): Record<string, unknown> | null => {
  try {
    const value: unknown = JSON.parse(credentialJSON.value)
    if (!value || Array.isArray(value) || typeof value !== 'object') throw new Error('invalid')
    return value as Record<string, unknown>
  } catch (_error) {
    Message.error('凭据必须是有效的 JSON 对象')
    return null
  }
}

const updateCredential = async (): Promise<boolean> => {
  if (!selectedBinding.value) return false
  const credential = parseCredential()
  if (!credential) return false
  try {
    runtime.value = (await adminPlatformAccountsApi.updateCredential(selectedBinding.value.id, credential)).data
    credentialJSON.value = ''
    Message.success('平台凭据已更新')
    await Promise.all([loadBindings(), loadDetail()])
    return true
  } catch (_error) {
    return false
  }
}

const changeGrant = async (consumer: string, enabled: boolean): Promise<void> => {
  if (!selectedBinding.value) return
  await adminPlatformAccountsApi.changeGrant(selectedBinding.value.id, consumer, { enabled })
  Message.success(enabled ? '应用授权已启用' : '应用授权已撤销')
  await loadDetail()
}

const enableConsumer = async (): Promise<void> => {
  const consumer = newConsumer.value.trim()
  if (!consumer) {
    Message.warning('请输入消费者 ID')
    return
  }
  grantSaving.value = true
  try {
    await changeGrant(consumer, true)
    newConsumer.value = ''
  } finally {
    grantSaving.value = false
  }
}

const removeBinding = async (): Promise<void> => {
  if (!selectedBinding.value) return
  removing.value = true
  try {
    await adminPlatformAccountsApi.remove(selectedBinding.value.id)
    Message.success('平台账号已删除')
    detailVisible.value = false
    await loadBindings()
  } finally {
    removing.value = false
  }
}

const statusColor = (status: string): string =>
  ({ active: 'green', credential_invalid: 'red', refresh_required: 'orange' })[status] ?? 'gray'

onMounted(() => void loadBindings())
</script>
