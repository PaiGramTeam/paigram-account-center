<template>
  <div class="user-management-page">
    <div class="mb-6">
      <page-header>
        <template #title>用户管理</template>
        <template #subtitle>管理系统中的所有用户账号</template>
      </page-header>
    </div>

    <UserTable
      ref="userTableRef"
      :user-api="userApi"
      :can-create="hasPermission('user:write')"
      :can-edit="hasPermission('user:write')"
      :can-delete="hasPermission('user:delete')"
      :can-batch-delete="hasPermission('user:delete')"
      :can-reset-password="hasPermission('user:manage')"
      :can-toggle-status="hasPermission('user:manage')"
      @view="handleViewUser"
      @edit="handleEditUser"
      @create="handleCreateUser"
      @delete="handleDeleteUser"
      @batch-delete="handleBatchDelete"
      @reset-password="handleResetPasswordFor"
      @toggle-status="handleToggleStatusFor"
    />

    <a-drawer v-model:visible="detailVisible" :width="600" :footer="false" unmount-on-close>
      <template #title>
        <span>用户详情</span>
      </template>

      <div v-if="currentUser">
        <UserCard :user="currentUser" :show-actions="false" class="mb-6" />

        <a-tabs default-active-key="info">
          <a-tab-pane key="info" title="基本信息">
            <a-descriptions :column="1" :label-style="{ width: '120px' }">
              <a-descriptions-item label="用户ID">
                {{ currentUser.id }}
              </a-descriptions-item>
              <a-descriptions-item label="用户名">
                {{ currentUser.primary_email }}
              </a-descriptions-item>
              <a-descriptions-item label="显示名称">
                {{ currentUser.display_name }}
              </a-descriptions-item>
              <a-descriptions-item label="邮箱">
                {{ currentUser.primary_email }}
                <a-tag v-if="hasVerifiedPrimaryEmail(currentUser)" color="green" size="small" class="ml-2">
                  已验证
                </a-tag>
              </a-descriptions-item>
              <a-descriptions-item label="手机号">
                {{ currentUser.phone || '-' }}
              </a-descriptions-item>
              <a-descriptions-item label="用户角色">
                <a-space wrap>
                  <a-tag v-for="role in currentUser.roles || []" :key="role">
                    {{ role }}
                  </a-tag>
                  <span v-if="!currentUser.roles?.length">-</span>
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="账号状态">
                <a-tag :color="getStatusColor(currentUser.status)">
                  {{ getStatusText(currentUser.status) }}
                </a-tag>
              </a-descriptions-item>
              <a-descriptions-item label="注册时间">
                {{ formatDate(currentUser.created_at) }}
              </a-descriptions-item>
              <a-descriptions-item label="最后登录">
                {{ formatDate(currentUser.last_login_at) }}
              </a-descriptions-item>
            </a-descriptions>
          </a-tab-pane>

          <a-tab-pane key="security" title="安全信息">
            <a-descriptions :column="1" :label-style="{ width: '120px' }">
              <a-descriptions-item label="登录方式">
                <a-space>
                  <a-tag v-for="method in currentUser.login_methods || []" :key="method">
                    {{ method }}
                  </a-tag>
                  <span v-if="!currentUser.login_methods?.length">-</span>
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="双因素认证">
                <a-tag :color="currentUser.two_factor_enabled ? 'green' : 'gray'">
                  {{ currentUser.two_factor_enabled ? '已启用' : '未启用' }}
                </a-tag>
              </a-descriptions-item>
              <a-descriptions-item label="登录IP">
                {{ currentUser.last_login_ip || '-' }}
              </a-descriptions-item>
              <a-descriptions-item label="登录设备">
                {{ currentUser.last_login_device || '-' }}
              </a-descriptions-item>
              <a-descriptions-item label="活跃会话">
                {{ currentUser.active_session_count ?? 0 }}
              </a-descriptions-item>
            </a-descriptions>

            <a-divider />

            <a-space>
              <a-button @click="handleResetPassword">重置密码</a-button>
              <a-button @click="handleForceLogout">强制登出</a-button>
            </a-space>
          </a-tab-pane>

          <a-tab-pane key="logs" title="操作日志">
            <a-timeline v-if="userLogs.length > 0">
              <a-timeline-item v-for="log in userLogs" :key="log.id">
                <div class="text-sm">
                  <div class="font-medium">{{ log.action }}</div>
                  <div class="text-gray-500">{{ formatDate(log.created_at) }} · {{ log.ip || '-' }}</div>
                  <div v-if="log.details" class="text-xs text-gray-500">{{ log.details }}</div>
                </div>
              </a-timeline-item>
            </a-timeline>
            <a-empty v-else description="暂无操作日志" />
          </a-tab-pane>
        </a-tabs>
      </div>
    </a-drawer>

    <a-modal
      v-model:visible="editVisible"
      :title="editMode === 'create' ? '新建用户' : '编辑用户'"
      :width="600"
      :on-before-ok="handleSaveUser"
    >
      <a-form ref="editFormRef" :model="editForm" :rules="editRules" layout="vertical">
        <a-form-item field="display_name" label="显示名称">
          <a-input v-model="editForm.display_name" placeholder="请输入显示名称" />
        </a-form-item>

        <a-form-item field="email" label="邮箱地址">
          <a-input v-model="editForm.email" placeholder="请输入邮箱地址" :disabled="editMode === 'edit'" />
        </a-form-item>

        <a-form-item v-if="editMode === 'create'" field="password" label="初始密码">
          <a-input-password v-model="editForm.password" placeholder="请输入初始密码" />
        </a-form-item>

        <a-form-item field="roles" label="用户角色">
          <a-select v-model="editForm.roles" placeholder="请选择用户角色" multiple allow-clear>
            <a-option v-for="role in roleOptions" :key="role.name" :value="role.name">
              {{ role.display_name }}
            </a-option>
          </a-select>
        </a-form-item>

        <a-form-item field="status" label="账号状态">
          <a-radio-group v-model="editForm.status">
            <a-radio value="active">正常</a-radio>
            <a-radio value="pending">待处理</a-radio>
            <a-radio value="suspended">已停用</a-radio>
            <a-radio value="deleted">已删除</a-radio>
          </a-radio-group>
        </a-form-item>
      </a-form>
    </a-modal>

    <ResetPasswordModal
      v-model:visible="resetPasswordVisible"
      :user-name="resetPasswordTarget?.display_name || ''"
      :reset-password="resetPasswordForTarget"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import type { FormInstance } from '@arco-design/web-vue'
import { ResetPasswordModal, UserTable, UserCard, PageHeader, useUserStore } from '@paigram/shared-components'
import { roleApi, userApi } from '@/api'
import type { RoleListItem, UserAuditLogItem, UserDetail, UserListItem, UserStatus } from '@paigram/shared-components'

const userStore = useUserStore()

const hasPermission = (permission: string): boolean => {
  const userPermissions = userStore.userInfo?.permissions || []
  return userPermissions.includes(permission)
}

// Refs
const userTableRef = ref<InstanceType<typeof UserTable>>()
const editFormRef = ref<FormInstance>()

const detailVisible = ref(false)
const currentUser = ref<UserDetail | null>(null)
const userLogs = ref<UserAuditLogItem[]>([])
const roleOptions = ref<RoleListItem[]>([])
const editingUserId = ref<number | null>(null)
const resetPasswordVisible = ref(false)
const resetPasswordTarget = ref<UserListItem | UserDetail | null>(null)

const editVisible = ref(false)
const editMode = ref<'create' | 'edit'>('create')
const editForm = reactive({
  display_name: '',
  email: '',
  password: '',
  roles: [] as string[],
  status: 'active' as UserStatus,
  locale: 'en_US',
})

const editRules = {
  display_name: [{ required: true, message: '请输入显示名称' }],
  email: [
    { required: true, message: '请输入邮箱地址' },
    { type: 'email', message: '请输入有效的邮箱地址' },
  ],
  password: [
    { required: true, message: '请输入初始密码' },
    { minLength: 8, message: '密码长度至少8位' },
  ],
  roles: [{ required: true, message: '请选择用户角色' }],
}

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

const formatDate = (date?: string | Date): string => {
  if (!date) return '-'
  return new Date(date).toLocaleString('zh-CN')
}

const loadRoleOptions = async () => {
  try {
    const response = await roleApi.getList({ page: 1, page_size: 100 })
    roleOptions.value = response.data
  } catch (error) {
    console.error('加载角色失败:', error)
    Message.error('加载角色列表失败')
  }
}

const loadUserDetail = async (userId: number | string): Promise<UserDetail> => {
  const [{ data: detail }, auditResponse, securityResponse] = await Promise.all([
    userApi.getDetail(userId),
    userApi.getAuditLogs(userId, { page: 1, page_size: 10 }),
    userApi.getSecuritySummary(userId),
  ])

  userLogs.value = auditResponse.data.data.data

  return {
    ...detail,
    login_methods: detail.login_methods || (detail.primary_login_type ? [detail.primary_login_type] : []),
    two_factor_enabled: securityResponse.data.two_factor_enabled,
    active_session_count: securityResponse.data.active_session_count,
    last_login_ip: securityResponse.data.last_login_ip,
    last_login_device: securityResponse.data.last_login_device,
  }
}

const hasVerifiedPrimaryEmail = (user: UserDetail) => {
  return !!user.emails?.find((email) => email.is_primary && email.verified_at)
}

const handleViewUser = async (user: UserListItem) => {
  try {
    currentUser.value = await loadUserDetail(user.id)
    detailVisible.value = true
  } catch (error) {
    console.error('加载用户详情失败:', error)
    Message.error('加载用户详情失败')
  }
}

const handleEditUser = async (user: UserListItem) => {
  try {
    const userDetail = await loadUserDetail(user.id)
    currentUser.value = userDetail
    editingUserId.value = user.id
    editMode.value = 'edit'
    editForm.display_name = userDetail.display_name
    editForm.email = userDetail.primary_email
    editForm.roles = [...(userDetail.roles || [])]
    editForm.status = userDetail.status
    editForm.locale = userDetail.locale || 'en_US'
    editVisible.value = true
  } catch (error) {
    console.error('加载编辑数据失败:', error)
    Message.error('加载用户数据失败')
  }
}

const handleCreateUser = () => {
  editingUserId.value = null
  editMode.value = 'create'
  editForm.display_name = ''
  editForm.email = ''
  editForm.password = ''
  editForm.roles = []
  editForm.status = 'active'
  editForm.locale = 'en_US'
  editVisible.value = true
}

const handleSaveUser = async (): Promise<boolean> => {
  const errors = await editFormRef.value?.validate()
  if (errors) return false

  try {
    if (editMode.value === 'create') {
      await userApi.create({
        email: editForm.email,
        display_name: editForm.display_name,
        password: editForm.password,
        roles: editForm.roles,
        status: editForm.status as 'active' | 'pending' | 'suspended' | 'deleted',
        locale: editForm.locale,
      })
      Message.success('创建成功')
    } else {
      if (editingUserId.value) {
        await userApi.update(editingUserId.value, {
          display_name: editForm.display_name,
          roles: editForm.roles,
          locale: editForm.locale,
        })
        if (currentUser.value?.status !== editForm.status) {
          await userApi.updateStatus(editingUserId.value, editForm.status)
        }
        Message.success('更新成功')
        if (currentUser.value?.id === editingUserId.value) {
          currentUser.value = await loadUserDetail(editingUserId.value)
        }
      }
    }
    userTableRef.value?.refresh()
    return true
  } catch (error) {
    console.error('保存失败:', error)
    const errorMessage = error instanceof Error ? error.message : '保存失败，请稍后重试'
    Message.error(errorMessage)
    return false
  }
}

const handleResetPasswordFor = (user: UserListItem | UserDetail): void => {
  if (!user?.id) return
  resetPasswordTarget.value = user
  resetPasswordVisible.value = true
}

const resetPasswordForTarget = async (password: string): Promise<void> => {
  if (!resetPasswordTarget.value) return
  try {
    await userApi.resetPassword(resetPasswordTarget.value.id, {
      new_password: password,
      invalidate_sessions: true,
    })
    Message.success('临时密码已设置，其他会话已撤销')
  } catch (error) {
    console.error('重置密码失败:', error)
    const errorMessage = error instanceof Error ? error.message : '重置密码失败'
    Message.error(errorMessage)
    throw error
  }
}

const handleResetPassword = () => {
  if (!currentUser.value) return
  handleResetPasswordFor(currentUser.value)
}

const handleForceLogout = async () => {
  if (!currentUser.value) return

  try {
    const response = await userApi.getSessions(currentUser.value.id, { page: 1, page_size: 100 })
    const sessions = response.data.data.data.filter((session) => !session.is_current)

    await Promise.all(sessions.map((session) => userApi.revokeSession(currentUser.value!.id, session.id)))

    currentUser.value = await loadUserDetail(currentUser.value.id)
    Message.success(sessions.length > 0 ? `已强制登出 ${sessions.length} 个会话` : '没有可强制登出的其他会话')
  } catch (error) {
    console.error('强制登出失败:', error)
    Message.error('强制登出失败')
  }
}

const handleDeleteUser = async (user: UserListItem) => {
  try {
    await userApi.delete(user.id)
    Message.success('删除成功')
    if (currentUser.value?.id === user.id) {
      detailVisible.value = false
      currentUser.value = null
      userLogs.value = []
    }
    userTableRef.value?.refresh()
  } catch (error) {
    console.error('删除失败:', error)
    Message.error(error instanceof Error ? error.message : '删除失败')
  }
}

const handleBatchDelete = async (users: UserListItem[]) => {
  try {
    await Promise.all(users.map((user) => userApi.delete(user.id)))
    Message.success(`已删除 ${users.length} 个用户`)
    userTableRef.value?.refresh()
  } catch (error) {
    console.error('批量删除失败:', error)
    Message.error(error instanceof Error ? error.message : '批量删除失败')
  }
}

const handleToggleStatusFor = async (user: UserListItem) => {
  const nextStatus: UserStatus = user.status === 'active' ? 'suspended' : 'active'
  try {
    await userApi.updateStatus(user.id, nextStatus)
    Message.success(nextStatus === 'active' ? '已激活用户' : '已停用用户')
    if (currentUser.value?.id === user.id) {
      currentUser.value = await loadUserDetail(user.id)
    }
    userTableRef.value?.refresh()
  } catch (error) {
    console.error('更新状态失败:', error)
    Message.error(error instanceof Error ? error.message : '更新状态失败')
  }
}

onMounted(() => {
  loadRoleOptions()
})
</script>
<style scoped></style>
