<template>
  <div class="space-y-6 p-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">账号活动</h1>
        <p class="text-sm text-gray-500">查看账号最近的安全与资料变更记录</p>
      </div>
    </div>

    <a-card>
      <a-form layout="inline" :model="filterForm">
        <a-form-item label="操作类型">
          <a-input
            v-model="filterForm.actionType"
            placeholder="例如 profile.update"
            allow-clear
            style="width: 220px"
            @press-enter="handleFilter"
          />
        </a-form-item>
        <a-form-item>
          <a-button @click="handleReset">重置</a-button>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card>
      <a-table
        :columns="columns"
        :data="logs"
        :loading="loading"
        :pagination="pagination"
        @page-change="handlePageChange"
        @page-size-change="handlePageSizeChange"
      >
        <template #created_at="{ record }">
          {{ formatDate(record.created_at) }}
        </template>

        <template #details="{ record }">{{ record.details || '-' }}</template>
      </a-table>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { useUserStore } from '@paigram/shared-components'
import { profileApi } from '@/api'
import type { ActivityLogItem } from '@paigram/shared-components'

const userStore = useUserStore()
const loading = ref(false)
const logs = ref<ActivityLogItem[]>([])
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)

const filterForm = reactive({
  actionType: '',
})

const columns = [
  { title: '时间', dataIndex: 'created_at', slotName: 'created_at', width: 180 },
  { title: '操作', dataIndex: 'action', width: 220 },
  { title: 'IP 地址', dataIndex: 'ip', width: 140 },
  { title: '详情', dataIndex: 'details', slotName: 'details' },
]

const pagination = computed(() => ({
  current: currentPage.value,
  pageSize: pageSize.value,
  total: total.value,
  showTotal: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50, 100],
}))

const formatDate = (date?: string): string => {
  if (!date) return '-'
  return new Date(date).toLocaleString('zh-CN')
}

const fetchLogs = async (): Promise<void> => {
  if (!userStore.userId) {
    Message.error('未找到用户信息')
    return
  }

  loading.value = true
  try {
    const response = await profileApi.getActivityLogs({
      page: currentPage.value,
      page_size: pageSize.value,
      action_type: filterForm.actionType || undefined,
    })

    logs.value = response.data.data.data
    total.value = response.data.data.pagination.total
  } catch (error) {
    console.error('加载账号活动失败:', error)
    Message.error('加载账号活动失败')
  } finally {
    loading.value = false
  }
}

const handleFilter = () => {
  currentPage.value = 1
  fetchLogs()
}

const handleReset = () => {
  filterForm.actionType = ''
  currentPage.value = 1
  fetchLogs()
}

const handlePageChange = (page: number) => {
  currentPage.value = page
  fetchLogs()
}

const handlePageSizeChange = (size: number) => {
  pageSize.value = size
  currentPage.value = 1
  fetchLogs()
}

onMounted(() => {
  fetchLogs()
})
</script>
