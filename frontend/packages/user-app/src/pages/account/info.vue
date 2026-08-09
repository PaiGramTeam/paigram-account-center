<template>
  <div class="space-y-6">
    <a-card title="基本信息" :loading="loading">
      <a-form ref="formRef" :model="form" :rules="rules" layout="vertical" @submit="handleSubmit">
        <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
          <!-- 头像 -->
          <a-form-item field="avatar_url" label="头像" class="col-span-full">
            <div class="flex items-center space-x-4">
              <a-avatar :size="80" :image-url="form.avatar_url || undefined">
                <template v-if="!form.avatar_url">
                  <icon-user />
                </template>
              </a-avatar>
              <div>
                <a-input v-model="form.avatar_url" placeholder="请输入头像 URL" class="mb-2" allow-clear />
                <div class="text-xs text-gray-500">请输入图片 URL，或使用第三方图床上传图片</div>
              </div>
            </div>
          </a-form-item>

          <!-- 显示名称 -->
          <a-form-item field="display_name" label="显示名称">
            <a-input v-model="form.display_name" placeholder="请输入显示名称" allow-clear />
          </a-form-item>

          <!-- 主邮箱（只读） -->
          <a-form-item label="主邮箱">
            <a-input :model-value="profileData?.primary_email" disabled>
              <template #suffix>
                <a-tag color="green" size="small">已验证</a-tag>
              </template>
            </a-input>
          </a-form-item>

          <!-- 语言偏好 -->
          <a-form-item field="locale" label="语言偏好">
            <a-select v-model="form.locale" placeholder="选择语言偏好" allow-clear>
              <a-option value="zh_CN">简体中文</a-option>
              <a-option value="en_US">English</a-option>
              <a-option value="ja_JP">日本語</a-option>
            </a-select>
          </a-form-item>

          <!-- 账号状态（只读） -->
          <a-form-item label="账号状态">
            <a-tag :color="getStatusColor(profileData?.status || 'active')">
              {{ getStatusText(profileData?.status || 'active') }}
            </a-tag>
          </a-form-item>

          <!-- 个人简介 -->
          <a-form-item field="bio" label="个人简介" class="col-span-full">
            <a-textarea
              v-model="form.bio"
              placeholder="介绍一下你自己..."
              :max-length="200"
              show-word-limit
              :auto-size="{ minRows: 3, maxRows: 6 }"
            />
          </a-form-item>
        </div>

        <div class="mt-6 flex justify-end space-x-4">
          <a-button @click="resetForm">重置</a-button>
          <a-button type="primary" html-type="submit" :loading="submitting">保存更改</a-button>
        </div>
      </a-form>
    </a-card>

    <!-- 账号详情 -->
    <a-card title="账号详情">
      <a-descriptions :column="{ xs: 1, sm: 2, md: 3 }" bordered>
        <a-descriptions-item label="用户ID">
          {{ profileData?.user_id || '-' }}
        </a-descriptions-item>
        <a-descriptions-item label="注册时间">
          {{ formatDate(profileData?.created_at) }}
        </a-descriptions-item>
        <a-descriptions-item label="最后登录">
          {{ formatDate(profileData?.last_login_at) }}
        </a-descriptions-item>
        <a-descriptions-item label="最后更新">
          {{ formatDate(profileData?.updated_at) }}
        </a-descriptions-item>
      </a-descriptions>
    </a-card>

    <!-- 邮箱列表 -->
    <a-card v-if="profileData?.emails && profileData.emails.length > 0" title="绑定邮箱">
      <a-list :data="profileData.emails" :bordered="false">
        <template #item="{ item }">
          <a-list-item>
            <div class="flex w-full items-center justify-between">
              <div class="flex items-center space-x-2">
                <icon-email class="text-lg text-gray-500" />
                <span>{{ item.email }}</span>
                <a-tag v-if="item.is_primary" color="blue" size="small">主邮箱</a-tag>
                <a-tag v-if="item.verified_at" color="green" size="small">已验证</a-tag>
                <a-tag v-else color="orange" size="small">未验证</a-tag>
              </div>
              <div v-if="item.verified_at" class="text-sm text-gray-500">验证于 {{ formatDate(item.verified_at) }}</div>
            </div>
          </a-list-item>
        </template>
      </a-list>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import { IconUser, IconEmail } from '@arco-design/web-vue/es/icon'
import type { FormInstance } from '@arco-design/web-vue'
import { useUserStore } from '@paigram/shared-components'
import { profileApi } from '@/api'
import type { ProfileData, UpdateProfileRequest } from '@paigram/shared-components'

const userStore = useUserStore()
const loading = ref(false)
const submitting = ref(false)
const formRef = ref<FormInstance>()
const profileData = ref<ProfileData | null>(null)

const form = reactive<UpdateProfileRequest>({
  display_name: '',
  avatar_url: '',
  bio: '',
  locale: '',
})

// 表单验证规则
const rules = {
  display_name: [
    { required: true, message: '请输入显示名称' },
    { minLength: 1, message: '显示名称不能为空' },
    { maxLength: 50, message: '显示名称不能超过50个字符' },
  ],
  bio: [{ maxLength: 200, message: '个人简介不能超过200个字符' }],
}

// 获取状态颜色
const getStatusColor = (status: string): string => {
  const colorMap: Record<string, string> = {
    active: 'green',
    pending: 'blue',
    suspended: 'red',
    deleted: 'gray',
  }
  return colorMap[status] || 'gray'
}

// 获取状态文本
const getStatusText = (status: string): string => {
  const textMap: Record<string, string> = {
    active: '正常',
    pending: '待处理',
    suspended: '已停用',
    deleted: '已删除',
  }
  return textMap[status] || '未知'
}

// 格式化日期
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

// 加载用户资料
const loadProfile = async (): Promise<void> => {
  const userId = userStore.userInfo?.id
  if (!userId) {
    Message.error('未获取到用户信息')
    return
  }

  loading.value = true
  try {
    const response = await profileApi.getProfile(userId)
    profileData.value = response.data

    // 填充表单
    form.display_name = response.data.display_name || ''
    form.avatar_url = response.data.avatar_url || ''
    form.bio = response.data.bio || ''
    form.locale = response.data.locale || ''
  } catch (error) {
    console.error('加载用户资料失败:', error)
    Message.error('加载用户资料失败')
  } finally {
    loading.value = false
  }
}

// 重置表单
const resetForm = (): void => {
  if (profileData.value) {
    form.display_name = profileData.value.display_name || ''
    form.avatar_url = profileData.value.avatar_url || ''
    form.bio = profileData.value.bio || ''
    form.locale = profileData.value.locale || ''
    formRef.value?.clearValidate()
  }
}

// 提交表单
const handleSubmit = async (): Promise<void> => {
  const valid = await formRef.value?.validate()
  if (!valid) return

  const userId = userStore.userInfo?.id
  if (!userId) {
    Message.error('未获取到用户信息')
    return
  }

  submitting.value = true
  try {
    // 只提交有变化的字段
    const updateData: UpdateProfileRequest = {}
    if (form.display_name && form.display_name !== profileData.value?.display_name) {
      updateData.display_name = form.display_name
    }
    if (form.avatar_url !== profileData.value?.avatar_url) {
      updateData.avatar_url = form.avatar_url
    }
    if (form.bio !== profileData.value?.bio) {
      updateData.bio = form.bio
    }
    if (form.locale !== profileData.value?.locale) {
      updateData.locale = form.locale
    }

    // 如果没有变化，提示用户
    if (Object.keys(updateData).length === 0) {
      Message.info('没有需要保存的更改')
      return
    }

    const response = await profileApi.updateProfile(userId, updateData)
    profileData.value = response.data

    // 更新 store 中的用户信息
    if (userStore.userInfo) {
      userStore.userInfo.display_name = response.data.display_name
      userStore.userInfo.avatar_url = response.data.avatar_url
    }

    Message.success('保存成功')
  } catch (error) {
    console.error('保存失败:', error)
    const errorMessage = error instanceof Error ? error.message : '保存失败，请稍后重试'
    Message.error(errorMessage)
  } finally {
    submitting.value = false
  }
}

// 组件挂载时加载数据
onMounted(() => {
  loadProfile()
})
</script>
