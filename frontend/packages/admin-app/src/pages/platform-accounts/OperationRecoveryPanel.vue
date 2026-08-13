<template>
  <a-alert class="mb-4" type="info">
    此处只恢复无载荷协调任务。原始凭据不会被保存或重放，系统会按原操作 ID 查询平台终态。
  </a-alert>
  <a-table :columns="columns" :data="items" row-key="operation_id" :loading="loading" :pagination="false">
    <template #operation="{ record }">
      <a-typography-text copyable>{{ record.operation_id }}</a-typography-text>
      <div class="mt-1 text-xs text-gray-500">{{ operationLabel(record.kind) }}</div>
    </template>
    <template #state="{ record }">
      <a-space direction="vertical" size="mini">
        <a-tag :color="stateColor(record.state)">{{ record.state }}</a-tag>
        <span v-if="record.reason_code" class="text-xs text-gray-500">{{ record.reason_code }}</span>
      </a-space>
    </template>
    <template #outbox="{ record }">
      <a-tag :color="outboxColor(record.outbox_status)">{{ record.outbox_status }}</a-tag>
      <div class="mt-1 text-xs text-gray-500">尝试 {{ record.attempt_count }} 次</div>
    </template>
    <template #updated="{ record }">{{ formatTime(record.updated_at) }}</template>
    <template #actions="{ record }">
      <a-popconfirm
        v-if="canRequeue && record.outbox_status === 'dead_letter'"
        content="确认按原操作参数重新进入协调队列？"
        type="warning"
        :ok-loading="requeueing === record.operation_id"
        @ok="requeue(record.operation_id)"
      >
        <a-button type="text" size="small">重新入队</a-button>
      </a-popconfirm>
      <span v-else class="text-xs text-gray-400">无需处理</span>
    </template>
  </a-table>
  <a-empty v-if="!loading && items.length === 0" description="暂无协调操作" />
  <a-pagination
    v-if="pagination.total > pagination.page_size"
    v-model:current="pagination.page"
    class="mt-4 justify-end"
    :page-size="pagination.page_size"
    :total="pagination.total"
    @change="loadOperations"
  />
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { Message } from '@arco-design/web-vue'
import { adminPlatformAccountsApi } from '@/api'
import type { OperationRecovery, Pagination } from '@paigram/shared-components'

const props = defineProps<{ bindingId: number; canRequeue: boolean }>()

const columns = [
  { title: '操作', slotName: 'operation', width: 230 },
  { title: '状态', slotName: 'state', width: 190 },
  { title: '队列', slotName: 'outbox', width: 130 },
  { title: '更新时间', slotName: 'updated', width: 170 },
  { title: '操作', slotName: 'actions', width: 100 },
]
const items = ref<OperationRecovery[]>([])
const loading = ref(false)
const requeueing = ref('')
const pagination = reactive<Pagination>({ page: 1, page_size: 10, total: 0, total_pages: 0 })

const loadOperations = async (): Promise<void> => {
  loading.value = true
  try {
    const response = await adminPlatformAccountsApi.listOperations(props.bindingId, {
      page: pagination.page,
      page_size: pagination.page_size,
    })
    items.value = response.data.items ?? []
    Object.assign(pagination, response.data.pagination)
  } finally {
    loading.value = false
  }
}

const requeue = async (operationId: string): Promise<void> => {
  requeueing.value = operationId
  try {
    await adminPlatformAccountsApi.requeueOperation(props.bindingId, operationId)
    Message.success('协调操作已重新入队')
    await loadOperations()
  } finally {
    requeueing.value = ''
  }
}

const operationLabel = (kind: string): string =>
  kind.replace('OPERATION_KIND_', '').toLocaleLowerCase().replace(/_/g, ' ')

const stateColor = (state: string): string =>
  ({ succeeded: 'green', failed: 'red', invariant_violation: 'orangered', input_required: 'orange' })[state] ?? 'blue'

const outboxColor = (status: string): string =>
  ({ completed: 'green', dead_letter: 'red', pending: 'orange' })[status] ?? 'gray'

const formatTime = (value: string): string =>
  new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value))

watch(
  () => props.bindingId,
  () => {
    pagination.page = 1
    void loadOperations()
  }
)
onMounted(() => void loadOperations())
</script>
