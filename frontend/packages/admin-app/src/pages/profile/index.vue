<template>
  <div class="p-5">
    <page-header>
      <template #title>个人资料</template>
      <template #subtitle>管理您的个人信息和账户设置</template>
    </page-header>

    <div class="mx-auto max-w-screen-lg">
      <a-card class="rounded-lg bg-white">
        <a-tabs default-active-key="basic">
          <a-tab-pane key="basic" title="基本信息">
            <a-form
              ref="profileFormRef"
              :model="profileForm"
              :rules="profileRules"
              label-align="left"
              :label-col-props="{ span: 6 }"
              :wrapper-col-props="{ span: 18 }"
            >
              <a-form-item label="头像">
                <div class="flex items-center space-x-4">
                  <a-avatar :size="64">
                    <img v-if="profileForm.avatar_url" :src="profileForm.avatar_url" alt="avatar" />
                    <icon-user v-else />
                  </a-avatar>
                  <a-button>更换头像</a-button>
                </div>
              </a-form-item>

              <a-form-item label="显示名称" field="display_name">
                <a-input v-model="profileForm.display_name" placeholder="请输入显示名称" />
              </a-form-item>

              <a-form-item label="邮箱">
                <a-input :model-value="userStore.userInfo?.primary_email" disabled />
              </a-form-item>

              <a-form-item label="简介" field="bio">
                <a-textarea
                  v-model="profileForm.bio"
                  placeholder="简单介绍一下自己"
                  :max-length="200"
                  show-word-limit
                  :auto-size="{ minRows: 3, maxRows: 5 }"
                />
              </a-form-item>

              <a-form-item label="语言偏好" field="locale">
                <a-select v-model="profileForm.locale" placeholder="选择语言">
                  <a-option value="zh-CN">简体中文</a-option>
                  <a-option value="en-US">English</a-option>
                </a-select>
              </a-form-item>

              <a-form-item>
                <a-space>
                  <a-button type="primary" :loading="saving" @click="handleSaveProfile">保存修改</a-button>
                  <a-button @click="handleResetForm">重置</a-button>
                </a-space>
              </a-form-item>
            </a-form>
          </a-tab-pane>

          <a-tab-pane key="security" title="安全设置">
            <div class="max-w-xl">
              <h3 class="mb-4 text-lg font-semibold">修改密码</h3>
              <a-form
                ref="passwordFormRef"
                :model="passwordForm"
                :rules="passwordRules"
                label-align="left"
                :label-col-props="{ span: 6 }"
                :wrapper-col-props="{ span: 18 }"
                class="max-w-lg"
              >
                <a-form-item label="当前密码" field="current_password">
                  <a-input-password v-model="passwordForm.current_password" placeholder="请输入当前密码" />
                </a-form-item>

                <a-form-item label="新密码" field="new_password">
                  <a-input-password v-model="passwordForm.new_password" placeholder="请输入新密码" />
                </a-form-item>

                <a-form-item label="确认密码" field="confirm_password">
                  <a-input-password v-model="passwordForm.confirm_password" placeholder="请再次输入新密码" />
                </a-form-item>

                <a-form-item>
                  <a-button type="primary" :loading="changingPassword" @click="handleChangePassword">
                    修改密码
                  </a-button>
                </a-form-item>
              </a-form>

              <a-divider />

              <h3 class="mb-4 text-lg font-semibold">两步验证</h3>
              <div class="flex items-center justify-between rounded bg-gray-50 p-4">
                <div>
                  <p class="font-medium">两步验证</p>
                  <p class="text-sm text-gray-500">为您的账户添加额外的安全保护</p>
                </div>
                <a-switch
                  v-model="twoFactorEnabled"
                  :loading="toggling2FA"
                  @change="(value: string | number | boolean) => handleToggle2FA(value as boolean)"
                />
              </div>
            </div>
          </a-tab-pane>

          <a-tab-pane key="sessions" title="登录会话">
            <div class="max-w-3xl">
              <h3 class="mb-4 text-lg font-semibold">活跃会话</h3>
              <a-list>
                <a-list-item v-for="session in activeSessions" :key="session.id">
                  <a-list-item-meta>
                    <template #title> {{ session.device }} - {{ session.browser }} </template>
                    <template #description>
                      <div class="text-sm text-gray-500">
                        <p>IP地址: {{ session.ip }}</p>
                        <p>最后活动: {{ formatDate(session.last_activity) }}</p>
                      </div>
                    </template>
                  </a-list-item-meta>
                  <template #actions>
                    <a-button
                      v-if="!session.is_current"
                      size="small"
                      status="danger"
                      @click="handleRevokeSession(session.id)"
                    >
                      注销
                    </a-button>
                    <a-tag v-else color="green">当前会话</a-tag>
                  </template>
                </a-list-item>
              </a-list>
            </div>
          </a-tab-pane>
        </a-tabs>
      </a-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import type { FormInstance } from '@arco-design/web-vue'
import { IconUser } from '@arco-design/web-vue/es/icon'
import { PageHeader, useUserStore } from '@paigram/shared-components'
import { profileApi } from '@/api'

const userStore = useUserStore()

// 表单引用
const profileFormRef = ref<FormInstance>()
const passwordFormRef = ref<FormInstance>()

// 加载状态
const saving = ref(false)
const changingPassword = ref(false)
const toggling2FA = ref(false)

// 基本信息表单
const profileForm = reactive({
  display_name: '',
  avatar_url: '',
  bio: '',
  locale: 'zh-CN',
})

// 密码表单
const passwordForm = reactive({
  current_password: '',
  new_password: '',
  confirm_password: '',
})

// 两步验证
const twoFactorEnabled = ref(false)

// 活跃会话
const activeSessions = ref([
  {
    id: '1',
    device: 'Windows PC',
    browser: 'Chrome 120.0',
    ip: '192.168.1.100',
    last_activity: new Date(),
    is_current: true,
  },
  {
    id: '2',
    device: 'iPhone 15',
    browser: 'Safari 17.0',
    ip: '192.168.1.101',
    last_activity: new Date(Date.now() - 3600000),
    is_current: false,
  },
])

// 表单验证规则
const profileRules = {
  display_name: [
    { required: true, message: '请输入显示名称' },
    { minLength: 2, maxLength: 50, message: '显示名称长度应在2-50个字符之间' },
  ],
  bio: [{ maxLength: 200, message: '简介不能超过200个字符' }],
}

const passwordRules = {
  current_password: [{ required: true, message: '请输入当前密码' }],
  new_password: [
    { required: true, message: '请输入新密码' },
    { minLength: 8, message: '密码长度至少8位' },
    {
      validator: (value: string, callback: Function) => {
        if (value && value === passwordForm.current_password) {
          callback('新密码不能与当前密码相同')
        } else {
          callback()
        }
      },
    },
  ],
  confirm_password: [
    { required: true, message: '请再次输入新密码' },
    {
      validator: (value: string, callback: Function) => {
        if (value !== passwordForm.new_password) {
          callback('两次输入的密码不一致')
        } else {
          callback()
        }
      },
    },
  ],
}

// 格式化日期
const formatDate = (date: Date): string => {
  return new Date(date).toLocaleString('zh-CN')
}

// 加载用户资料
const loadProfile = async () => {
  try {
    if (!userStore.userInfo?.id) return
    const { data } = await profileApi.getProfile(userStore.userInfo.id)
    profileForm.display_name = data.display_name
    profileForm.avatar_url = data.avatar_url || ''
    profileForm.bio = data.bio || ''
    profileForm.locale = data.locale || 'zh-CN'
  } catch (_error) {
    Message.error('加载个人资料失败')
  }
}

// 保存个人资料
const handleSaveProfile = async () => {
  const valid = await profileFormRef.value?.validate()
  if (!valid) return

  saving.value = true
  try {
    if (!userStore.userInfo?.id) {
      Message.error('用户信息未加载')
      return
    }
    await profileApi.updateProfile(userStore.userInfo.id, {
      display_name: profileForm.display_name,
      avatar_url: profileForm.avatar_url,
      bio: profileForm.bio,
      locale: profileForm.locale,
    })
    Message.success('个人资料更新成功')
    userStore.fetchUserInfo()
  } catch (_error) {
    Message.error('更新失败，请稍后重试')
  } finally {
    saving.value = false
  }
}

// 重置表单
const handleResetForm = () => {
  loadProfile()
}

// 修改密码
const handleChangePassword = async () => {
  const valid = await passwordFormRef.value?.validate()
  if (!valid) return

  changingPassword.value = true
  try {
    // TODO: 调用修改密码API
    Message.success('密码修改成功，请重新登录')
    // 清空表单
    passwordForm.current_password = ''
    passwordForm.new_password = ''
    passwordForm.confirm_password = ''
  } catch (_error) {
    Message.error('密码修改失败，请检查当前密码是否正确')
  } finally {
    changingPassword.value = false
  }
}

// 切换两步验证
const handleToggle2FA = async (value: boolean) => {
  toggling2FA.value = true
  try {
    // TODO: 调用API切换两步验证
    Message.success(value ? '两步验证已开启' : '两步验证已关闭')
  } catch (_error) {
    Message.error('操作失败，请稍后重试')
    twoFactorEnabled.value = !value
  } finally {
    toggling2FA.value = false
  }
}

// 注销会话
const handleRevokeSession = async (sessionId: string) => {
  try {
    // TODO: 调用API注销会话
    Message.success('会话已注销')
    activeSessions.value = activeSessions.value.filter((s) => s.id !== sessionId)
  } catch (_error) {
    Message.error('注销失败，请稍后重试')
  }
}

onMounted(() => {
  loadProfile()
})
</script>
