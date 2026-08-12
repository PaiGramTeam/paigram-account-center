<template>
  <div class="space-y-4 p-6">
    <div>
      <h1 class="text-2xl font-semibold">审计日志</h1>
      <p class="mt-1 text-gray-500">查询权限、平台账号和系统配置等真实审计事件。</p>
    </div>
    <a-card>
      <a-form :model="filters" layout="inline">
        <a-form-item label="分类"><a-input v-model="filters.category" allow-clear /></a-form-item>
        <a-form-item label="结果">
          <a-select v-model="filters.result" allow-clear style="width: 140px">
            <a-option value="success">成功</a-option>
            <a-option value="failure">失败</a-option>
          </a-select>
        </a-form-item>
        <a-button type="primary" @click="search">查询</a-button>
      </a-form>
    </a-card>
    <a-card>
      <a-table
        :columns="columns"
        :data="events"
        row-key="id"
        :loading="loading"
        :pagination="pagination"
        @page-change="changePage"
      >
        <template #actor="{ record }"
          >{{ record.actor_type }}{{ record.actor_user_id ? ` #${record.actor_user_id}` : '' }}</template
        >
        <template #target="{ record }"
          >{{ record.target_type || '-' }}{{ record.target_id ? ` #${record.target_id}` : '' }}</template
        >
        <template #result="{ record }"
          ><a-tag :color="record.result === 'success' ? 'green' : 'red'">{{ record.result }}</a-tag></template
        >
        <template #created_at="{ record }">{{ formatDate(record.created_at) }}</template>
      </a-table>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { adminSystemApi } from '@/api'
import type { AuditEvent } from '@paigram/shared-components'

const events = ref<AuditEvent[]>([])
const loading = ref(false)
const currentPage = ref(1)
const total = ref(0)
const filters = reactive({ category: '', result: '' })
const columns = [
  { title: '时间', slotName: 'created_at', width: 180 },
  { title: '分类', dataIndex: 'category', width: 130 },
  { title: '操作', dataIndex: 'action' },
  { title: '执行者', slotName: 'actor', width: 140 },
  { title: '目标', slotName: 'target', width: 160 },
  { title: '结果', slotName: 'result', width: 100 },
  { title: '请求 ID', dataIndex: 'request_id', width: 220 },
]
const pagination = computed(() => ({ current: currentPage.value, pageSize: 20, total: total.value }))

const loadEvents = async (): Promise<void> => {
  loading.value = true
  try {
    const response = await adminSystemApi.listAuditEvents({
      page: currentPage.value,
      page_size: 20,
      category: filters.category || undefined,
      result: filters.result || undefined,
    })
    events.value = response.data.items ?? []
    total.value = response.data.pagination.total
  } finally {
    loading.value = false
  }
}
const search = (): void => {
  currentPage.value = 1
  void loadEvents()
}
const changePage = (page: number): void => {
  currentPage.value = page
  void loadEvents()
}
const formatDate = (value: string): string => new Date(value).toLocaleString('zh-CN')

onMounted(() => void loadEvents())
</script>
