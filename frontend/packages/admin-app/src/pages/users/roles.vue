<template>
  <div class="p-6">
    <div class="mb-6 flex items-center justify-between">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">角色管理</h1>
      <a-button v-if="hasPermission('role:create')" type="primary" @click="handleCreate">
        <template #icon>
          <icon-plus />
        </template>
        创建角色
      </a-button>
    </div>

    <a-card :bordered="false" class="shadow-sm">
      <a-spin v-if="loading" class="flex min-h-[400px] items-center justify-center" />

      <a-table
        v-else
        :columns="columns"
        :data="roleList"
        :pagination="paginationConfig"
        :loading="loading"
        @page-change="handlePageChange"
        @page-size-change="handlePageSizeChange"
      >
        <template #name="{ record }">
          <div class="flex flex-col">
            <span class="font-medium text-gray-900 dark:text-white">{{ record.display_name }}</span>
            <span class="text-sm text-gray-500 dark:text-gray-400">{{ record.name }}</span>
          </div>
        </template>

        <template #description="{ record }">
          <span class="text-gray-600 dark:text-gray-300">{{ record.description || '-' }}</span>
        </template>

        <template #permission_count="{ record }">
          <a-tag color="blue">{{ record.permission_count }} 个权限</a-tag>
        </template>

        <template #user_count="{ record }">
          <a-tag color="green">{{ record.user_count ?? '-' }} 个用户</a-tag>
        </template>

        <template #system="{ record }">
          <a-tag :color="record.is_system ? 'gold' : 'gray'">
            {{ record.is_system ? '系统角色' : '自定义角色' }}
          </a-tag>
        </template>

        <template #created_at="{ record }">
          <span class="text-sm text-gray-500 dark:text-gray-400">{{ formatDateTime(record.created_at) }}</span>
        </template>

        <template #actions="{ record }">
          <a-space>
            <a-button v-if="hasPermission('role:manage')" type="text" size="small" @click="openAssignments(record)">
              分配
            </a-button>
            <a-button v-if="hasPermission('role:update')" type="text" size="small" @click="handleEdit(record)">
              <template #icon>
                <icon-edit />
              </template>
              编辑
            </a-button>
            <a-button
              v-if="hasPermission('role:delete')"
              type="text"
              size="small"
              status="danger"
              @click="handleDelete(record)"
            >
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
      :title="modalMode === 'create' ? '创建角色' : '编辑角色'"
      :mask-closable="false"
      @ok="handleModalOk"
      @cancel="handleModalCancel"
    >
      <a-form :model="formData" layout="vertical">
        <a-form-item label="角色名称" :rules="[{ required: true, message: '请输入角色名称' }]" field="name">
          <a-input
            v-model="formData.name"
            :disabled="modalMode === 'edit'"
            placeholder="请输入角色名称（英文，如 admin）"
          />
        </a-form-item>
        <a-form-item label="显示名称" :rules="[{ required: true, message: '请输入显示名称' }]" field="display_name">
          <a-input v-model="formData.display_name" placeholder="请输入显示名称（如管理员）" />
        </a-form-item>
        <a-form-item label="角色描述" field="description">
          <a-textarea v-model="formData.description" placeholder="请输入角色描述" :rows="3" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-drawer
      :visible="assignmentVisible"
      :width="680"
      :title="assignmentRole ? `分配：${assignmentRole.display_name}` : '角色分配'"
      @cancel="assignmentVisible = false"
    >
      <a-spin :loading="assignmentLoading" class="w-full">
        <a-tabs>
          <a-tab-pane key="permissions" title="权限">
            <a-checkbox-group v-model="assignedPermissionIds" class="grid grid-cols-1 gap-3 md:grid-cols-2">
              <a-checkbox v-for="permission in allPermissions" :key="permission.id" :value="permission.id">
                <div>
                  <div class="font-medium">{{ permission.name }}</div>
                  <div class="text-xs text-gray-500">{{ permission.description || '无描述' }}</div>
                </div>
              </a-checkbox>
            </a-checkbox-group>
            <a-empty v-if="allPermissions.length === 0" description="暂无权限" />
          </a-tab-pane>
          <a-tab-pane key="users" title="用户">
            <a-select v-model="assignedUserIds" multiple allow-search allow-clear placeholder="请选择此角色的用户">
              <a-option v-for="user in allUsers" :key="user.id" :value="user.id">
                {{ user.display_name }} · #{{ user.id }}
              </a-option>
            </a-select>
          </a-tab-pane>
        </a-tabs>
      </a-spin>
      <template #footer>
        <a-space>
          <a-button @click="assignmentVisible = false">取消</a-button>
          <a-button type="primary" :loading="assignmentSaving" @click="saveAssignments">保存分配</a-button>
        </a-space>
      </template>
    </a-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { Message, Modal } from '@arco-design/web-vue'
import { IconPlus, IconEdit, IconDelete } from '@arco-design/web-vue/es/icon'
import { permissionApi, roleApi, userApi } from '@/api'
import {
  useUserStore,
  type CreateRoleRequest,
  type PermissionListItem,
  type RoleListItem,
  type UpdateRoleRequest,
  type UserListItem,
} from '@paigram/shared-components'

const userStore = useUserStore()

const columns = [
  {
    title: '角色名称',
    dataIndex: 'name',
    slotName: 'name',
    width: 200,
  },
  {
    title: '描述',
    dataIndex: 'description',
    slotName: 'description',
  },
  {
    title: '权限数量',
    dataIndex: 'permission_count',
    slotName: 'permission_count',
    width: 120,
  },
  {
    title: '用户数量',
    dataIndex: 'user_count',
    slotName: 'user_count',
    width: 120,
  },
  {
    title: '类型',
    dataIndex: 'is_system',
    slotName: 'system',
    width: 120,
  },
  {
    title: '创建时间',
    dataIndex: 'created_at',
    slotName: 'created_at',
    width: 180,
  },
  {
    title: '操作',
    slotName: 'actions',
    width: 150,
  },
]

const loading = ref(false)
const roleList = ref<RoleListItem[]>([])
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)

const modalVisible = ref(false)
const modalMode = ref<'create' | 'edit'>('create')
const currentEditingRole = ref<RoleListItem | null>(null)
const assignmentVisible = ref(false)
const assignmentLoading = ref(false)
const assignmentSaving = ref(false)
const assignmentRole = ref<RoleListItem | null>(null)
const allPermissions = ref<PermissionListItem[]>([])
const allUsers = ref<UserListItem[]>([])
const assignedPermissionIds = ref<number[]>([])
const assignedUserIds = ref<number[]>([])

const formData = reactive({
  name: '',
  display_name: '',
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

const hasPermission = (permission: string): boolean => userStore.hasPermission(permission)

const formatDateTime = (dateString: string): string => {
  if (!dateString) return '-'
  const date = new Date(dateString)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const loadRoleList = async (): Promise<void> => {
  loading.value = true
  try {
    const { data, pagination } = await roleApi.getList({
      page: currentPage.value,
      page_size: pageSize.value,
    })
    roleList.value = data
    total.value = pagination.total
  } catch (error) {
    const message = error instanceof Error ? error.message : '加载角色列表失败'
    Message.error(message)
    console.error('Failed to load role list:', error)
  } finally {
    loading.value = false
  }
}

const handlePageChange = (page: number): void => {
  currentPage.value = page
  loadRoleList()
}

const handlePageSizeChange = (size: number): void => {
  pageSize.value = size
  currentPage.value = 1
  loadRoleList()
}

const handleCreate = (): void => {
  modalMode.value = 'create'
  currentEditingRole.value = null
  formData.name = ''
  formData.display_name = ''
  formData.description = ''
  modalVisible.value = true
}

const handleEdit = (role: RoleListItem): void => {
  modalMode.value = 'edit'
  currentEditingRole.value = role
  formData.name = role.name
  formData.display_name = role.display_name
  formData.description = role.description
  modalVisible.value = true
}

const handleDelete = (role: RoleListItem): void => {
  if (role.is_system) {
    Message.warning('系统角色不允许删除')
    return
  }

  Modal.confirm({
    title: '删除角色',
    content: `确定要删除角色 "${role.display_name}" 吗？此操作不可恢复。`,
    okText: '确定',
    cancelText: '取消',
    onOk: async () => {
      try {
        await roleApi.delete(role.id)
        Message.success('删除角色成功')
        await loadRoleList()
      } catch (error) {
        const message = error instanceof Error ? error.message : '删除角色失败'
        Message.error(message)
        console.error('Failed to delete role:', error)
      }
    },
  })
}

const handleModalOk = async (): Promise<void> => {
  try {
    if (modalMode.value === 'create') {
      const createData: CreateRoleRequest = {
        name: formData.name,
        display_name: formData.display_name,
        description: formData.description,
      }
      await roleApi.create(createData)
      Message.success('创建角色成功')
    } else {
      if (!currentEditingRole.value) return
      const updateData: UpdateRoleRequest = {
        display_name: formData.display_name,
        description: formData.description,
      }
      await roleApi.update(currentEditingRole.value.id, updateData)
      Message.success('更新角色成功')
    }
    modalVisible.value = false
    await loadRoleList()
  } catch (error) {
    const message = error instanceof Error ? error.message : '操作失败'
    Message.error(message)
    console.error('Failed to save role:', error)
  }
}

const handleModalCancel = (): void => {
  modalVisible.value = false
}

const openAssignments = async (role: RoleListItem): Promise<void> => {
  assignmentRole.value = role
  assignmentVisible.value = true
  assignmentLoading.value = true
  try {
    const [catalog, assignedPermissions, users, assignedUsers] = await Promise.all([
      permissionApi.getList({ page: 1, page_size: 1000 }),
      roleApi.getPermissions(role.id),
      userApi.getList({ page: 1, page_size: 100 }),
      roleApi.getUsers(role.id),
    ])
    allPermissions.value = catalog.data
    assignedPermissionIds.value = (assignedPermissions.data ?? []).map((permission) => permission.id)
    allUsers.value = users.data
    assignedUserIds.value = (assignedUsers.data ?? []).map((user) => user.id)
  } finally {
    assignmentLoading.value = false
  }
}

const saveAssignments = async (): Promise<void> => {
  if (!assignmentRole.value) return
  assignmentSaving.value = true
  try {
    await Promise.all([
      roleApi.replacePermissions(assignmentRole.value.id, assignedPermissionIds.value),
      roleApi.replaceUsers(assignmentRole.value.id, assignedUserIds.value),
    ])
    Message.success('角色分配已更新')
    assignmentVisible.value = false
    await loadRoleList()
  } finally {
    assignmentSaving.value = false
  }
}

onMounted(async () => {
  await loadRoleList()
})
</script>
