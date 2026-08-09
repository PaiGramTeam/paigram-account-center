<template>
  <div class="user-detail-page p-6">
    <div class="mb-6">
      <a-page-header :show-back="true" title="用户详情" subtitle="查看和管理用户详细信息" @back="handleBack">
        <template #extra>
          <a-space>
            <a-button @click="handleEdit">编辑用户</a-button>
            <a-button status="danger" @click="handleDelete">删除用户</a-button>
          </a-space>
        </template>
      </a-page-header>
    </div>

    <a-spin :loading="loading" class="w-full">
      <div v-if="userDetail" class="space-y-6">
        <a-card>
          <div class="flex items-start space-x-6">
            <a-avatar :size="100" :image-url="userDetail.avatar_url || undefined">
              <template v-if="!userDetail.avatar_url">
                <icon-user class="text-4xl" />
              </template>
            </a-avatar>
            <div class="flex-1">
              <div class="mb-2 flex items-center space-x-3">
                <h2 class="text-2xl font-bold">{{ userDetail.display_name }}</h2>
                <a-tag :color="getStatusColor(userDetail.status)">
                  {{ getStatusText(userDetail.status) }}
                </a-tag>
              </div>
              <div class="space-y-1 text-gray-600">
                <div class="flex items-center space-x-2">
                  <icon-email />
                  <span>{{ userDetail.primary_email }}</span>
                </div>
                <div v-if="userDetail.bio" class="flex items-center space-x-2">
                  <icon-info-circle />
                  <span>{{ userDetail.bio }}</span>
                </div>
              </div>
              <div class="mt-4 flex space-x-4 text-sm text-gray-500">
                <span>用户ID: {{ userDetail.id }}</span>
                <span>注册于 {{ formatDate(userDetail.created_at) }}</span>
                <span v-if="userDetail.last_login_at"> 最后登录 {{ formatDate(userDetail.last_login_at) }} </span>
              </div>
            </div>
          </div>
        </a-card>

        <a-row :gutter="16">
          <a-col :span="12">
            <a-card title="基本信息">
              <a-descriptions :column="1" bordered>
                <a-descriptions-item label="显示名称">
                  {{ userDetail.display_name }}
                </a-descriptions-item>
                <a-descriptions-item label="主邮箱">
                  {{ userDetail.primary_email }}
                </a-descriptions-item>
                <a-descriptions-item label="语言偏好">
                  {{ userDetail.locale || '-' }}
                </a-descriptions-item>
                <a-descriptions-item label="主登录方式">
                  <a-tag v-if="userDetail.primary_login_type">
                    {{ userDetail.primary_login_type }}
                  </a-tag>
                  <span v-else>-</span>
                </a-descriptions-item>
                <a-descriptions-item label="注册时间">
                  {{ formatDate(userDetail.created_at) }}
                </a-descriptions-item>
                <a-descriptions-item label="最后更新">
                  {{ formatDate(userDetail.updated_at) }}
                </a-descriptions-item>
                <a-descriptions-item label="最后登录">
                  {{ formatDate(userDetail.last_login_at) }}
                </a-descriptions-item>
                <a-descriptions-item label="用户角色">
                  <a-space wrap>
                    <a-tag v-for="role in userDetail.roles || []" :key="role">{{ role }}</a-tag>
                    <span v-if="!userDetail.roles?.length">-</span>
                  </a-space>
                </a-descriptions-item>
              </a-descriptions>
            </a-card>
          </a-col>

          <a-col :span="12">
            <a-card title="绑定邮箱">
              <a-list
                v-if="userDetail.emails && userDetail.emails.length > 0"
                :data="userDetail.emails"
                :bordered="false"
              >
                <template #item="{ item }">
                  <a-list-item>
                    <div class="flex w-full items-center justify-between">
                      <div class="flex items-center space-x-2">
                        <icon-email class="text-gray-500" />
                        <span>{{ item.email }}</span>
                        <a-tag v-if="item.is_primary" color="blue" size="small">主邮箱</a-tag>
                      </div>
                      <a-tag v-if="item.verified_at" color="green" size="small"> 已验证 </a-tag>
                      <a-tag v-else color="orange" size="small">未验证</a-tag>
                    </div>
                  </a-list-item>
                </template>
              </a-list>
              <a-empty v-else description="暂无绑定邮箱" />
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="16">
          <a-col :span="12">
            <a-card title="安全概览">
              <a-descriptions :column="1" bordered>
                <a-descriptions-item label="双因素认证">
                  <a-tag :color="userDetail.two_factor_enabled ? 'green' : 'gray'">
                    {{ userDetail.two_factor_enabled ? '已启用' : '未启用' }}
                  </a-tag>
                </a-descriptions-item>
                <a-descriptions-item label="活跃会话数">
                  {{ userDetail.active_session_count ?? 0 }}
                </a-descriptions-item>
                <a-descriptions-item label="最近登录 IP">
                  {{ securitySummary?.last_login_ip || '-' }}
                </a-descriptions-item>
                <a-descriptions-item label="最近登录设备">
                  {{ securitySummary?.last_login_device || '-' }}
                </a-descriptions-item>
                <a-descriptions-item label="近 30 天失败登录">
                  {{ securitySummary?.failed_logins_last_30_days ?? 0 }}
                </a-descriptions-item>
              </a-descriptions>
            </a-card>
          </a-col>

          <a-col :span="12">
            <a-card title="有效权限">
              <a-space v-if="userDetail.permissions?.length" wrap>
                <a-tag v-for="permission in userDetail.permissions" :key="permission">
                  {{ permission }}
                </a-tag>
              </a-space>
              <a-empty v-else description="暂无权限数据" />
            </a-card>
          </a-col>
        </a-row>

        <a-card title="活跃会话">
          <a-list v-if="sessions.length > 0" :data="sessions" :bordered="false">
            <template #item="{ item }">
              <a-list-item>
                <div class="flex w-full items-center justify-between gap-4">
                  <div>
                    <div class="font-medium">{{ item.device_name || item.device_type || '未知设备' }}</div>
                    <div class="text-sm text-gray-500">
                      {{ item.ip || '-' }} · {{ item.location || '未知位置' }} ·
                      {{ formatDate(item.last_active_at || item.created_at) }}
                    </div>
                  </div>
                  <a-space>
                    <a-tag v-if="item.is_current" color="green">当前会话</a-tag>
                    <a-button v-else type="text" status="danger" @click="handleRevokeSession(item.id)">踢下线</a-button>
                  </a-space>
                </div>
              </a-list-item>
            </template>
          </a-list>
          <a-empty v-else description="暂无活跃会话" />
        </a-card>

        <a-card title="账号管理">
          <a-space size="large">
            <a-button type="primary" @click="handleResetPassword">重置密码</a-button>
            <a-button @click="handleToggleStatus">
              {{ userDetail.status === 'active' ? '停用账号' : '激活账号' }}
            </a-button>
          </a-space>
        </a-card>
      </div>

      <a-empty v-else description="用户不存在" />
    </a-spin>

    <a-modal v-model:visible="editVisible" title="编辑用户" :width="600" :on-before-ok="handleSaveEdit">
      <a-form ref="editFormRef" :model="editForm" :rules="editRules" layout="vertical">
        <a-form-item field="display_name" label="显示名称">
          <a-input v-model="editForm.display_name" placeholder="请输入显示名称" />
        </a-form-item>

        <a-form-item field="status" label="账号状态">
          <a-radio-group v-model="editForm.status">
            <a-radio value="active">正常</a-radio>
            <a-radio value="pending">待处理</a-radio>
            <a-radio value="suspended">已停用</a-radio>
            <a-radio value="deleted">已删除</a-radio>
          </a-radio-group>
        </a-form-item>

        <a-form-item field="locale" label="语言偏好">
          <a-select v-model="editForm.locale" placeholder="选择语言偏好" allow-clear>
            <a-option value="zh_CN">简体中文</a-option>
            <a-option value="en_US">English</a-option>
            <a-option value="ja_JP">日本語</a-option>
          </a-select>
        </a-form-item>
      </a-form>
    </a-modal>

    <ResetPasswordModal
      v-model:visible="resetPasswordVisible"
      :user-name="userDetail?.display_name || ''"
      :reset-password="resetCurrentUserPassword"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message, Modal } from '@arco-design/web-vue'
import { IconUser, IconEmail, IconInfoCircle } from '@arco-design/web-vue/es/icon'
import type { FormInstance } from '@arco-design/web-vue'
import { userApi } from '@/api'
import { ResetPasswordModal } from '@paigram/shared-components'
import type {
  UpdateUserRequest,
  UserDetail,
  UserSecuritySummary,
  UserSessionItem,
  UserStatus,
} from '@paigram/shared-components'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const userDetail = ref<UserDetail | null>(null)
const securitySummary = ref<UserSecuritySummary | null>(null)
const sessions = ref<UserSessionItem[]>([])
const editVisible = ref(false)
const resetPasswordVisible = ref(false)
const editFormRef = ref<FormInstance>()

const editForm = reactive<UpdateUserRequest & { status: UserStatus }>({
  display_name: '',
  status: 'active',
  locale: '',
})

const editRules = {
  display_name: [
    { required: true, message: '请输入显示名称' },
    { minLength: 1, message: '显示名称不能为空' },
  ],
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

const formatDate = (date?: string): string => {
  if (!date) return '-'
  return new Date(date).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const loadUserDetail = async (): Promise<void> => {
  const userId = route.params.id
  if (!userId) {
    Message.error('用户ID无效')
    return
  }

  loading.value = true
  try {
    const [{ data: detail }, securityResponse, sessionsResponse] = await Promise.all([
      userApi.getDetail(userId as string),
      userApi.getSecuritySummary(userId as string),
      userApi.getSessions(userId as string, { page: 1, page_size: 100 }),
    ])

    userDetail.value = {
      ...detail,
      two_factor_enabled: securityResponse.data.two_factor_enabled,
      active_session_count: securityResponse.data.active_session_count,
    }
    securitySummary.value = securityResponse.data
    sessions.value = sessionsResponse.data.data.data
  } catch (error) {
    console.error('加载用户详情失败:', error)
    Message.error('加载用户详情失败')
  } finally {
    loading.value = false
  }
}

const handleBack = (): void => {
  router.push('/users/list')
}

const handleEdit = (): void => {
  if (!userDetail.value) return

  editForm.display_name = userDetail.value.display_name
  editForm.status = userDetail.value.status as 'active' | 'pending' | 'suspended' | 'deleted'
  editForm.locale = userDetail.value.locale || ''
  editVisible.value = true
}

const handleSaveEdit = async (): Promise<boolean> => {
  const errors = await editFormRef.value?.validate()
  if (errors) return false

  const userId = route.params.id
  if (!userId) return false

  try {
    await userApi.update(userId as string, {
      display_name: editForm.display_name,
      locale: editForm.locale,
    })
    if (userDetail.value?.status !== editForm.status) {
      await userApi.updateStatus(userId as string, editForm.status)
    }
    Message.success('更新成功')
    await loadUserDetail()
    return true
  } catch (error) {
    console.error('更新失败:', error)
    const errorMessage = error instanceof Error ? error.message : '更新失败，请稍后重试'
    Message.error(errorMessage)
    return false
  }
}

const handleDelete = (): void => {
  if (!userDetail.value) return

  Modal.confirm({
    title: '确认删除',
    content: `确定要删除用户 "${userDetail.value.display_name}" 吗？此操作不可恢复。`,
    okText: '确定',
    cancelText: '取消',
    okButtonProps: { status: 'danger' },
    onOk: async () => {
      const userId = route.params.id
      if (!userId) return

      try {
        await userApi.delete(userId as string)
        Message.success('删除成功')
        router.push('/users/list')
      } catch (error) {
        console.error('删除失败:', error)
        const errorMessage = error instanceof Error ? error.message : '删除失败，请稍后重试'
        Message.error(errorMessage)
      }
    },
  })
}

const handleResetPassword = (): void => {
  if (!userDetail.value) return
  resetPasswordVisible.value = true
}

const resetCurrentUserPassword = async (password: string): Promise<void> => {
  const userId = route.params.id
  if (!userId) return
  try {
    await userApi.resetPassword(userId as string, { new_password: password, invalidate_sessions: true })
    Message.success('临时密码已设置，其他会话已撤销')
  } catch (error) {
    console.error('重置密码失败:', error)
    const errorMessage = error instanceof Error ? error.message : '重置密码失败'
    Message.error(errorMessage)
    throw error
  }
}

const handleToggleStatus = (): void => {
  if (!userDetail.value) return

  const newStatus = userDetail.value.status === 'active' ? 'suspended' : 'active'
  const action = newStatus === 'active' ? '激活' : '停用'

  Modal.confirm({
    title: `${action}用户`,
    content: `确定要${action}用户 "${userDetail.value.display_name}" 吗？`,
    okText: '确定',
    cancelText: '取消',
    onOk: async () => {
      const userId = route.params.id
      if (!userId) return

      try {
        await userApi.updateStatus(userId as string, newStatus)
        Message.success(`${action}成功`)
        await loadUserDetail()
      } catch (error) {
        console.error(`${action}失败:`, error)
        const errorMessage = error instanceof Error ? error.message : `${action}失败`
        Message.error(errorMessage)
      }
    },
  })
}

const handleRevokeSession = (sessionId: number): void => {
  if (!userDetail.value) return

  Modal.confirm({
    title: '踢出会话',
    content: `确定要踢出用户 "${userDetail.value.display_name}" 的这个会话吗？`,
    okText: '确定',
    cancelText: '取消',
    okButtonProps: { status: 'danger' },
    onOk: async () => {
      try {
        await userApi.revokeSession(userDetail.value!.id, sessionId)
        Message.success('已踢出该会话')
        await loadUserDetail()
      } catch (error) {
        console.error('踢出会话失败:', error)
        Message.error(error instanceof Error ? error.message : '踢出会话失败')
      }
    },
  })
}

onMounted(() => {
  loadUserDetail()
})
</script>
