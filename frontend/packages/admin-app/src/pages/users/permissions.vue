<template>
  <div class="p-6">
    <div class="mb-6 flex items-center justify-between">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">权限管理</h1>
      <a-button type="primary" @click="handleCreate">
        <template #icon>
          <icon-plus />
        </template>
        创建权限
      </a-button>
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

        <template #actions="{ record }">
          <a-space>
            <a-button type="text" size="small" status="danger" @click="handleDelete(record)">
              <template #icon>
                <icon-delete />
              </template>
              删除
            </a-button>
          </a-space>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:visible="modalVisible"
      title="创建权限"
      :mask-closable="false"
      @ok="handleModalOk"
      @cancel="handleModalCancel"
    >
      <a-form :model="formData" layout="vertical">
        <a-form-item label="权限名称" :rules="[{ required: true, message: '请输入权限名称' }]" field="name">
          <a-input v-model="formData.name" placeholder="请输入权限名称（如 user:read）" />
          <template #extra>
            <span class="text-xs text-gray-500">推荐格式：资源:动作，例如 user:read、role:write</span>
          </template>
        </a-form-item>

        <a-form-item label="资源" :rules="[{ required: true, message: '请选择资源' }]" field="resource">
          <a-select v-model="formData.resource" placeholder="请选择资源">
            <a-option v-for="resource in resourceOptions" :key="resource.value" :value="resource.value">
              {{ resource.label }}
            </a-option>
          </a-select>
        </a-form-item>

        <a-form-item label="动作" :rules="[{ required: true, message: '请输入动作' }]" field="action">
          <a-input v-model="formData.action" placeholder="请输入动作，如 read、write、delete、manage" />
        </a-form-item>

        <a-form-item label="权限描述" field="description">
          <a-textarea v-model="formData.description" placeholder="请输入权限描述" :rows="3" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Message, Modal } from '@arco-design/web-vue'
import { IconDelete, IconPlus } from '@arco-design/web-vue/es/icon'
import { permissionApi } from '@/api'
import type { CreatePermissionRequest, PermissionListItem } from '@paigram/shared-components'

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
  { title: '操作', slotName: 'actions', width: 120 },
]

const loading = ref(false)
const permissionList = ref<PermissionListItem[]>([])
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)

const filterForm = reactive({
  category: '',
})

const modalVisible = ref(false)
const formData = reactive({
  name: '',
  resource: '',
  action: '',
  description: '',
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

watch(
  () => [formData.resource, formData.action],
  ([resource, action]) => {
    if (resource && action) {
      formData.name = `${resource}:${action}`
    }
  }
)

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

const handleCreate = (): void => {
  formData.name = ''
  formData.resource = ''
  formData.action = ''
  formData.description = ''
  modalVisible.value = true
}

const handleDelete = (permission: PermissionListItem): void => {
  Modal.confirm({
    title: '删除权限',
    content: `确定要删除权限 "${permission.name}" 吗？此操作不可恢复。`,
    okText: '确定',
    cancelText: '取消',
    onOk: async () => {
      try {
        await permissionApi.delete(permission.id)
        Message.success('删除权限成功')
        await loadPermissionList()
      } catch (error) {
        const message = error instanceof Error ? error.message : '删除权限失败'
        Message.error(message)
      }
    },
  })
}

const handleModalOk = async (): Promise<void> => {
  try {
    const createData: CreatePermissionRequest = {
      name: formData.name,
      resource: formData.resource,
      action: formData.action,
      description: formData.description,
    }
    await permissionApi.create(createData)
    Message.success('创建权限成功')
    modalVisible.value = false
    await loadPermissionList()
  } catch (error) {
    const message = error instanceof Error ? error.message : '创建权限失败'
    Message.error(message)
  }
}

const handleModalCancel = (): void => {
  modalVisible.value = false
}

onMounted(async () => {
  await loadPermissionList()
})
</script>
