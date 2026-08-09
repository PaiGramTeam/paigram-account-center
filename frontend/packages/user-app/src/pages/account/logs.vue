<template>
  <div class="space-y-6 p-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">登录日志</h1>
        <p class="text-sm text-gray-500">查看账号最近的登录记录和失败尝试</p>
      </div>
    </div>

    <a-card>
      <a-form layout="inline" :model="filterForm">
        <a-form-item label="状态">
          <a-select
            v-model="filterForm.status"
            placeholder="全部状态"
            allow-clear
            style="width: 160px"
            @change="handleFilter"
          >
            <a-option value="success">成功</a-option>
            <a-option value="failed">失败</a-option>
          </a-select>
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
        <template #status="{ record }">
          <a-tag :color="record.status === 'success' ? 'green' : 'red'">
            {{ record.status === 'success' ? '成功' : '失败' }}
          </a-tag>
        </template>

        <template #device="{ record }">
          <div>
            <div class="font-medium">{{ record.device || '未知设备' }}</div>
            <div class="text-xs text-gray-500">{{ record.user_agent || '-' }}</div>
          </div>
        </template>

        <template #created_at="{ record }">
          {{ formatDate(record.created_at) }}
        </template>

        <template #extra="{ record }">
          <div>
            <div>{{ record.location || '-' }}</div>
            <div v-if="record.failure_reason" class="text-xs text-red-500">{{ record.failure_reason }}</div>
          </div>
        </template>
      </a-table>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { useUserStore } from '@paigram/shared-components'
import { userApi } from '@/api'
import type { UserLoginLogItem } from '@paigram/shared-components'

const userStore = useUserStore()
const loading = ref(false)
const logs = ref<UserLoginLogItem[]>([])
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)

const filterForm = reactive({
  status: undefined as 'success' | 'failed' | undefined,
})

const columns = [
  { title: '时间', dataIndex: 'created_at', slotName: 'created_at', width: 180 },
  { title: '状态', dataIndex: 'status', slotName: 'status', width: 100 },
  { title: '登录方式', dataIndex: 'login_type', width: 120 },
  { title: 'IP 地址', dataIndex: 'ip', width: 140 },
  { title: '设备', dataIndex: 'device', slotName: 'device', width: 260 },
  { title: '附加信息', dataIndex: 'extra', slotName: 'extra' },
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
    const response = await userApi.getLoginLogs(userStore.userId, {
      page: currentPage.value,
      page_size: pageSize.value,
      status: filterForm.status,
    })

    logs.value = response.data.data.data
    total.value = response.data.data.pagination.total
  } catch (error) {
    console.error('加载登录日志失败:', error)
    Message.error('加载登录日志失败')
  } finally {
    loading.value = false
  }
}

const handleFilter = () => {
  currentPage.value = 1
  fetchLogs()
}

const handleReset = () => {
  filterForm.status = undefined
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
