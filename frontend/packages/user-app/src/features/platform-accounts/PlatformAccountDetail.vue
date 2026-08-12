<template>
  <a-drawer :visible="visible" :width="760" title="平台账号详情" @cancel="emit('update:visible', false)">
    <a-spin :loading="loading" class="w-full">
      <a-descriptions v-if="binding" :column="2" bordered>
        <a-descriptions-item label="名称">{{ binding.display_name }}</a-descriptions-item>
        <a-descriptions-item label="状态"
          ><a-tag :color="statusColor(binding.status)">{{ binding.status }}</a-tag></a-descriptions-item
        >
        <a-descriptions-item label="平台">{{ binding.platform }}</a-descriptions-item>
        <a-descriptions-item label="外部账号">{{ binding.external_account_key || '-' }}</a-descriptions-item>
        <a-descriptions-item label="运行状态">{{ runtime?.status || '-' }}</a-descriptions-item>
        <a-descriptions-item label="最后验证">{{ formatDate(runtime?.last_validated_at) }}</a-descriptions-item>
      </a-descriptions>

      <a-space class="my-4" wrap>
        <a-button type="primary" :loading="refreshing" @click="refreshCredential">刷新凭据</a-button>
        <a-button @click="credentialVisible = true">更换凭据</a-button>
        <a-popconfirm content="删除后平台凭据也会被清理，确认继续？" type="warning" @ok="removeBinding">
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
                <template #actions>
                  <a-tag v-if="item.is_primary" color="green">主角色</a-tag>
                  <a-button v-else type="text" @click="setPrimary(item.id)">设为主角色</a-button>
                </template>
              </a-list-item>
            </template>
          </a-list>
          <a-empty v-if="profiles.length === 0" description="暂无角色" />
        </a-tab-pane>
        <a-tab-pane key="grants" title="应用授权">
          <div class="mb-4 flex gap-2">
            <a-input v-model="newConsumer" placeholder="输入应用提供的消费者 ID" @press-enter="enableConsumer" />
            <a-button type="primary" :loading="grantSaving" @click="enableConsumer">授权应用</a-button>
          </div>
          <div v-for="consumer in consumers" :key="consumer" class="mb-3 rounded border border-gray-200 p-4">
            <div class="flex items-center justify-between">
              <div>
                <div class="font-medium">{{ consumer }}</div>
                <div class="mt-1 text-xs text-gray-500">{{ grantActions(consumer).join('、') || '未授权' }}</div>
              </div>
              <a-switch
                :model-value="grantEnabled(consumer)"
                @change="(value) => changeGrant(consumer, Boolean(value))"
              />
            </div>
          </div>
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
import { computed, ref, watch } from 'vue'
import { Message } from '@arco-design/web-vue'
import { platformAccountsApi } from '@/api'
import type {
  ConsumerGrant,
  PlatformBinding,
  PlatformProfile,
  PlatformRuntimeSummary,
} from '@paigram/shared-components'

const props = defineProps<{ visible: boolean; binding: PlatformBinding | null }>()
const emit = defineEmits<{ 'update:visible': [value: boolean]; 'changed': []; 'deleted': [] }>()
const loading = ref(false)
const refreshing = ref(false)
const removing = ref(false)
const credentialVisible = ref(false)
const credentialJSON = ref('')
const newConsumer = ref('')
const grantSaving = ref(false)
const profiles = ref<PlatformProfile[]>([])
const grants = ref<ConsumerGrant[]>([])
const runtime = ref<PlatformRuntimeSummary | null>(null)
const consumers = computed(() => Array.from(new Set(grants.value.map((grant) => grant.consumer))))

const loadDetail = async (): Promise<void> => {
  if (!props.binding) return
  loading.value = true
  try {
    const [profileResponse, grantResponse, runtimeResponse] = await Promise.all([
      platformAccountsApi.listProfiles(props.binding.id),
      platformAccountsApi.listGrants(props.binding.id),
      platformAccountsApi.getRuntimeSummary(props.binding.id),
    ])
    profiles.value = profileResponse.data.items ?? []
    grants.value = grantResponse.data.items ?? []
    runtime.value = runtimeResponse.data
  } catch (_error) {
    Message.error('加载平台账号详情失败')
  } finally {
    loading.value = false
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
  if (!props.binding) return false
  const credential = parseCredential()
  if (!credential) return false
  try {
    runtime.value = (await platformAccountsApi.updateCredential(props.binding.id, credential)).data
    credentialJSON.value = ''
    Message.success('平台凭据已更新')
    emit('changed')
    return true
  } catch (_error) {
    return false
  }
}

const refreshCredential = async (): Promise<void> => {
  if (!props.binding) return
  refreshing.value = true
  try {
    runtime.value = (await platformAccountsApi.refresh(props.binding.id)).data
    Message.success('平台账号已刷新')
    emit('changed')
    await loadDetail()
  } finally {
    refreshing.value = false
  }
}

const setPrimary = async (profileId: number): Promise<void> => {
  if (!props.binding) return
  await platformAccountsApi.setPrimaryProfile(props.binding.id, profileId)
  Message.success('主角色已更新')
  await loadDetail()
  emit('changed')
}

const changeGrant = async (consumer: string, enabled: boolean): Promise<void> => {
  if (!props.binding) return
  await platformAccountsApi.changeGrant(props.binding.id, consumer, { enabled })
  Message.success(enabled ? '应用授权已启用' : '应用授权已撤销')
  await loadDetail()
  emit('changed')
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
  if (!props.binding) return
  removing.value = true
  try {
    await platformAccountsApi.remove(props.binding.id)
    Message.success('平台账号已删除')
    emit('update:visible', false)
    emit('deleted')
  } finally {
    removing.value = false
  }
}

const grantEnabled = (consumer: string): boolean =>
  grants.value.some((grant) => grant.consumer === consumer && grant.status === 'active')
const grantActions = (consumer: string): string[] =>
  grants.value.find((grant) => grant.consumer === consumer)?.actions ?? []
const statusColor = (status: string): string =>
  ({ active: 'green', credential_invalid: 'red', refresh_required: 'orange' })[status] ?? 'gray'
const formatDate = (value: unknown): string =>
  typeof value === 'string' ? new Date(value).toLocaleString('zh-CN') : '-'

watch(
  () => [props.visible, props.binding?.id] as const,
  ([visible]) => {
    if (visible) void loadDetail()
  }
)
</script>
