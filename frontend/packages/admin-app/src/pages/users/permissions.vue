<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">权限概览</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">权限由后端路由定义，在角色管理中完成分配。</p>
    </div>

    <a-card :bordered="false" class="mb-4 shadow-sm">
      <a-form layout="inline" :model="filterForm">
        <a-form-item label="资源类型" field="category">
          <a-select
            v-model="filterForm.category"
            placeholder="选择资源类型"
            allow-clear
            style="width: 220px"
            @change="handleFilter"
          >
            <a-option value="">全部资源</a-option>
            <a-option v-for="resource in resourceOptions" :key="resource.value" :value="resource.value">
              {{ resource.label }}
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item>
          <a-button @click="handleReset">重置</a-button>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card :bordered="false" class="shadow-sm">
      <a-spin v-if="loading" class="flex min-h-[400px] items-center justify-center" />

      <a-table
        v-else
        :columns="columns"
        :data="permissionList"
        :pagination="paginationConfig"
        :loading="loading"
        @page-change="handlePageChange"
        @page-size-change="handlePageSizeChange"
      >
        <template #name="{ record }">
          <div class="flex flex-col">
            <span class="font-medium text-gray-900 dark:text-white">{{ record.display_name }}</span>
            <code class="text-xs text-gray-500 dark:text-gray-400">{{ record.name }}</code>
          </div>
        </template>

        <template #resource="{ record }">
          <a-tag :color="getResourceColor(record.resource)">
            {{ getResourceText(record.resource) }}
          </a-tag>
        </template>

        <template #action="{ record }">
          <a-tag color="arcoblue">{{ record.action }}</a-tag>
        </template>

        <template #description="{ record }">
          <span class="text-gray-600 dark:text-gray-300">{{ record.description || '-' }}</span>
        </template>
      </a-table>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { permissionApi } from '@/api'
import type { PermissionListItem } from '@paigram/shared-components'

const resourceOptions = [
  { value: 'user', label: '用户' },
  { value: 'role', label: '角色' },
  { value: 'permission', label: '权限' },
  { value: 'audit', label: '审计' },
  { value: 'session', label: '会话' },
  { value: 'bot', label: '机器人' },
]

const columns = [
  { title: '权限名称', dataIndex: 'name', slotName: 'name', width: 260 },
  { title: '资源', dataIndex: 'resource', slotName: 'resource', width: 140 },
  { title: '动作', dataIndex: 'action', slotName: 'action', width: 120 },
  { title: '描述', dataIndex: 'description', slotName: 'description' },
]

const loading = ref(false)
const permissionList = ref<PermissionListItem[]>([])
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)

const filterForm = reactive({
  category: '',
})

const paginationConfig = computed(() => ({
  current: currentPage.value,
  pageSize: pageSize.value,
  total: total.value,
  showTotal: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50, 100],
}))

const getResourceColor = (resource: string): string => {
  const colorMap: Record<string, string> = {
    user: 'blue',
    role: 'green',
    permission: 'orange',
    audit: 'red',
    session: 'purple',
    bot: 'cyan',
  }
  return colorMap[resource] || 'gray'
}

const getResourceText = (resource: string): string => {
  return resourceOptions.find((item) => item.value === resource)?.label || resource
}

const loadPermissionList = async (): Promise<void> => {
  loading.value = true
  try {
    const { data, pagination } = await permissionApi.getList({
      page: currentPage.value,
      page_size: pageSize.value,
      category: filterForm.category || undefined,
    })
    permissionList.value = data
    total.value = pagination.total
  } catch (error) {
    const message = error instanceof Error ? error.message : '加载权限列表失败'
    Message.error(message)
  } finally {
    loading.value = false
  }
}

const handlePageChange = (page: number): void => {
  currentPage.value = page
  loadPermissionList()
}

const handlePageSizeChange = (size: number): void => {
  pageSize.value = size
  currentPage.value = 1
  loadPermissionList()
}

const handleFilter = (): void => {
  currentPage.value = 1
  loadPermissionList()
}

const handleReset = (): void => {
  filterForm.category = ''
  currentPage.value = 1
  loadPermissionList()
}

onMounted(async () => {
  await loadPermissionList()
})
</script>
