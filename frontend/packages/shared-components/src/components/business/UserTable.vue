<template>
  <div class="user-table-container">
    <a-card v-if="showSearch" class="mb-4">
      <a-form :model="searchForm" layout="inline">
        <a-row :gutter="16" class="w-full">
          <a-col :span="6">
            <a-form-item field="keyword" label="关键词">
              <a-input
                v-model="searchForm.keyword"
                placeholder="搜索用户名、邮箱"
                allow-clear
                @press-enter="handleSearch"
              />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item field="status" label="状态">
              <a-select v-model="searchForm.status" placeholder="选择状态" allow-clear>
                <a-option value="active">正常</a-option>
                <a-option value="pending">待处理</a-option>
                <a-option value="suspended">已停用</a-option>
                <a-option value="deleted">已删除</a-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item field="dateRange" label="注册时间">
              <a-range-picker v-model="searchForm.dateRange" :shortcuts="dateShortcuts" />
            </a-form-item>
          </a-col>
          <a-col :span="6" class="text-right">
            <a-space>
              <a-button @click="handleReset">重置</a-button>
              <a-button type="primary" @click="handleSearch">
                <template #icon>
                  <icon-search />
                </template>
                搜索
              </a-button>
            </a-space>
          </a-col>
        </a-row>
      </a-form>
    </a-card>

    <a-card>
      <div v-if="showActions" class="mb-4 flex items-center justify-between">
        <a-space>
          <a-button v-if="canCreate" type="primary" @click="handleCreate">
            <template #icon>
              <icon-plus />
            </template>
            新建用户
          </a-button>
          <a-button v-if="selectedKeys.length > 0 && canBatchDelete" status="danger" @click="handleBatchDelete">
            <template #icon>
              <icon-delete />
            </template>
            批量删除 ({{ selectedKeys.length }})
          </a-button>
        </a-space>

        <a-space>
          <a-button @click="handleExport">
            <template #icon>
              <icon-export />
            </template>
            导出
          </a-button>
          <a-button @click="handleRefresh">
            <template #icon>
              <icon-refresh />
            </template>
            刷新
          </a-button>
        </a-space>
      </div>

      <a-table
        v-model:selected-keys="selectedKeys"
        :columns="tableColumns"
        :data="tableData"
        :pagination="pagination"
        :loading="loading"
        :row-selection="rowSelection"
        :scroll="{ x: 1200 }"
        @page-change="handlePageChange"
        @page-size-change="handlePageSizeChange"
      >
        <template #user="{ record }">
          <div class="flex items-center space-x-3">
            <a-avatar :size="32">
              <img v-if="record.avatar_url" :src="record.avatar_url" alt="avatar" />
              <icon-user v-else />
            </a-avatar>
            <div>
              <div class="font-medium">{{ record.display_name }}</div>
            </div>
          </div>
        </template>

        <template #status="{ record }">
          <a-tag :color="getStatusColor(record.status)">
            {{ getStatusText(record.status) }}
          </a-tag>
        </template>

        <template #roles="{ record }">
          <a-space wrap>
            <a-tag v-for="role in record.roles" :key="role">
              {{ role }}
            </a-tag>
          </a-space>
        </template>

        <template #action="{ record }">
          <a-space>
            <a-button v-if="canView" type="text" size="small" @click="handleView(record)"> 查看 </a-button>
            <a-button v-if="canEdit" type="text" size="small" @click="handleEdit(record)"> 编辑 </a-button>
            <a-dropdown v-if="canDelete || canResetPassword || canToggleStatus">
              <a-button type="text" size="small">
                更多
                <icon-down />
              </a-button>
              <template #content>
                <a-doption v-if="canResetPassword" @click="handleResetPassword(record)"> 重置密码 </a-doption>
                <a-doption v-if="canToggleStatus" @click="handleToggleStatus(record)">
                  {{ record.status === 'active' ? '停用' : '激活' }}
                </a-doption>
                <a-doption v-if="canDelete" class="text-red-500" @click="handleDelete(record)"> 删除 </a-doption>
              </template>
            </a-dropdown>
          </a-space>
        </template>
      </a-table>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { Message, Modal } from '@arco-design/web-vue'
import type { TableColumnData, TableRowSelection } from '@arco-design/web-vue'
import {
  IconSearch,
  IconPlus,
  IconDelete,
  IconExport,
  IconRefresh,
  IconUser,
  IconDown,
} from '@arco-design/web-vue/es/icon'
import type { UserListItem, UserListParams, UserListResponse } from '../../api/types'

interface Props {
  userApi: {
    getList: (params?: UserListParams) => Promise<UserListResponse>
  }

  showSearch?: boolean
  showActions?: boolean

  canView?: boolean
  canEdit?: boolean
  canCreate?: boolean
  canDelete?: boolean
  canBatchDelete?: boolean
  canResetPassword?: boolean
  canToggleStatus?: boolean

  pageSize?: number
  columns?: string[]
}

const props = withDefaults(defineProps<Props>(), {
  showSearch: true,
  showActions: true,
  canView: true,
  canEdit: true,
  canCreate: true,
  canDelete: true,
  canBatchDelete: true,
  canResetPassword: true,
  canToggleStatus: true,
  pageSize: 20,
  columns: () => ['user', 'status', 'roles', 'created_at', 'action'],
})

const emit = defineEmits<{
  'view': [user: UserListItem]
  'edit': [user: UserListItem]
  'create': []
  'delete': [user: UserListItem]
  'batch-delete': [users: UserListItem[]]
  'reset-password': [user: UserListItem]
  'toggle-status': [user: UserListItem]
  'refresh': []
}>()

const searchForm = reactive({
  keyword: '',
  status: undefined,
  dateRange: undefined,
})

const loading = ref(false)
const tableData = ref<UserListItem[]>([])
const selectedKeys = ref<(string | number)[]>([])

const pagination = reactive({
  current: 1,
  pageSize: props.pageSize,
  total: 0,
  showTotal: true,
  showJumper: true,
  showPageSize: true,
})

const dateShortcuts = [
  {
    label: '最近7天',
    value: () => {
      const end = new Date()
      const start = new Date()
      start.setDate(start.getDate() - 7)
      return [start, end]
    },
  },
  {
    label: '最近30天',
    value: () => {
      const end = new Date()
      const start = new Date()
      start.setDate(start.getDate() - 30)
      return [start, end]
    },
  },
]

const rowSelection = computed<TableRowSelection | undefined>(() => {
  if (!props.canBatchDelete) return undefined

  return {
    type: 'checkbox',
    showCheckedAll: true,
    onlyCurrent: false,
  }
})

const tableColumns = computed<TableColumnData[]>(() => {
  const allColumns: Record<string, TableColumnData> = {
    user: {
      title: '用户',
      slotName: 'user',
      width: 250,
    },
    status: {
      title: '状态',
      slotName: 'status',
      width: 100,
    },
    roles: {
      title: '角色',
      slotName: 'roles',
      width: 200,
    },
    created_at: {
      title: '注册时间',
      dataIndex: 'created_at',
      width: 180,
      render: ({ record }: { record: UserListItem }) => formatDate(record.created_at),
    },
    last_login_at: {
      title: '最后登录',
      dataIndex: 'last_login_at',
      width: 180,
      render: ({ record }: { record: UserListItem }) => formatDate(record.last_login_at),
    },
    action: {
      title: '操作',
      slotName: 'action',
      fixed: 'right',
      width: 150,
    },
  }

  return props.columns.map((col) => allColumns[col]).filter(Boolean)
})

const getStatusColor = (status: string): string => {
  const colorMap: Record<string, string> = {
    active: 'green',
    pending: 'blue',
    suspended: 'red',
    deleted: 'gray',
  }
  return colorMap[status] || 'gray'
}

const getStatusText = (status: string): string => {
  const textMap: Record<string, string> = {
    active: '正常',
    pending: '待处理',
    suspended: '已停用',
    deleted: '已删除',
  }
  return textMap[status] || '未知'
}

const formatDate = (date?: string): string => {
  if (!date) return '-'
  return new Date(date).toLocaleString('zh-CN')
}

const fetchUsers = async () => {
  loading.value = true
  try {
    const params = {
      page: pagination.current,
      page_size: pagination.pageSize,
      search: searchForm.keyword || undefined,
      status: searchForm.status,
    }

    const response = await props.userApi.getList(params)
    tableData.value = response.data
    pagination.total = response.pagination.total
  } catch (_error) {
    Message.error('获取用户列表失败')
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  pagination.current = 1
  fetchUsers()
}

const handleReset = () => {
  searchForm.keyword = ''
  searchForm.status = undefined
  searchForm.dateRange = undefined
  handleSearch()
}

const handleRefresh = () => {
  emit('refresh')
  fetchUsers()
}

const handlePageChange = (page: number) => {
  pagination.current = page
  fetchUsers()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.pageSize = pageSize
  pagination.current = 1
  fetchUsers()
}

const handleView = (record: UserListItem) => {
  emit('view', record)
}

const handleEdit = (record: UserListItem) => {
  emit('edit', record)
}

const handleCreate = () => {
  emit('create')
}

const handleDelete = (record: UserListItem) => {
  Modal.confirm({
    title: '确认删除',
    content: `确定要删除用户 "${record.display_name}" 吗？`,
    okText: '确定',
    cancelText: '取消',
    okButtonProps: { status: 'danger' },
    onOk: () => {
      emit('delete', record)
    },
  })
}

const handleBatchDelete = () => {
  const selectedUsers = tableData.value.filter((user) => selectedKeys.value.includes(user.id))

  Modal.confirm({
    title: '批量删除确认',
    content: `确定要删除选中的 ${selectedUsers.length} 个用户吗？`,
    okText: '确定',
    cancelText: '取消',
    okButtonProps: { status: 'danger' },
    onOk: () => {
      emit('batch-delete', selectedUsers)
    },
  })
}

const handleResetPassword = (record: UserListItem) => {
  emit('reset-password', record)
}

const handleToggleStatus = (record: UserListItem) => {
  const action = record.status === 'active' ? '停用' : '激活'
  Modal.confirm({
    title: `${action}用户`,
    content: `确定要${action}用户 "${record.display_name}" 吗？`,
    okText: '确定',
    cancelText: '取消',
    onOk: () => {
      emit('toggle-status', record)
    },
  })
}

const handleExport = () => {
  Message.info('导出功能开发中...')
}

onMounted(() => {
  fetchUsers()
})

defineExpose({
  refresh: fetchUsers,
})
</script>
