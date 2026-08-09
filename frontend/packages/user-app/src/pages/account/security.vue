<template>
  <div class="space-y-6">
    <!-- 修改密码 -->
    <a-card title="修改密码" :bordered="false" class="shadow-sm">
      <a-form :model="passwordForm" layout="vertical" @submit="handleChangePassword">
        <a-form-item label="当前密码" field="old_password" :rules="[{ required: true, message: '请输入当前密码' }]">
          <a-input-password
            v-model="passwordForm.old_password"
            placeholder="请输入当前密码"
            :disabled="passwordLoading"
          />
        </a-form-item>
        <a-form-item
          label="新密码"
          field="new_password"
          :rules="[
            { required: true, message: '请输入新密码' },
            { minLength: 8, message: '密码长度至少 8 位' },
          ]"
        >
          <a-input-password
            v-model="passwordForm.new_password"
            placeholder="请输入新密码（至少 8 位）"
            :disabled="passwordLoading"
          />
        </a-form-item>
        <a-form-item
          label="确认新密码"
          field="confirm_password"
          :rules="[
            { required: true, message: '请再次输入新密码' },
            {
              validator: (value: string, cb: (error?: string) => void) => {
                if (value !== passwordForm.new_password) {
                  cb('两次输入的密码不一致')
                } else {
                  cb()
                }
              },
            },
          ]"
        >
          <a-input-password
            v-model="passwordForm.confirm_password"
            placeholder="请再次输入新密码"
            :disabled="passwordLoading"
          />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" html-type="submit" :loading="passwordLoading">修改密码</a-button>
        </a-form-item>
      </a-form>
    </a-card>

    <!-- 双因素认证 (2FA) -->
    <a-card title="双因素认证 (2FA)" :bordered="false" class="shadow-sm">
      <div class="space-y-4">
        <div class="flex items-center justify-between rounded-lg bg-gray-50 p-4 dark:bg-gray-800">
          <div>
            <p class="font-medium text-gray-900 dark:text-white">
              双因素认证{{ twoFactorEnabled ? '已启用' : '未启用' }}
            </p>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ twoFactorEnabled ? '您的账号已开启双因素认证，登录时需要验证码' : '启用双因素认证可以提高账号安全性' }}
            </p>
          </div>
          <a-button v-if="!twoFactorEnabled" type="primary" :loading="twoFactorLoading" @click="handleEnable2FA">
            启用 2FA
          </a-button>
          <a-button v-else status="danger" :loading="twoFactorLoading" @click="showDisable2FAModal">
            禁用 2FA
          </a-button>
        </div>
      </div>
    </a-card>

    <a-card title="安全概览" :bordered="false" class="shadow-sm">
      <a-descriptions :column="{ xs: 1, sm: 2, md: 4 }" bordered>
        <a-descriptions-item label="活跃会话">
          {{ securitySummary?.active_session_count ?? 0 }}
        </a-descriptions-item>
        <a-descriptions-item label="设备数量">
          {{ securitySummary?.device_count ?? 0 }}
        </a-descriptions-item>
        <a-descriptions-item label="近 30 天失败登录">
          {{ securitySummary?.failed_logins_last_30_days ?? 0 }}
        </a-descriptions-item>
        <a-descriptions-item label="最近登录">
          {{ formatAbsoluteDate(securitySummary?.last_login_at) }}
        </a-descriptions-item>
      </a-descriptions>
      <div class="mt-4 text-sm text-gray-500">
        最近登录 IP：{{ securitySummary?.last_login_ip || '-' }}
        <span class="mx-2">·</span>
        最近登录设备：{{ securitySummary?.last_login_device || '-' }}
      </div>
    </a-card>

    <!-- 登录设备管理 -->
    <a-card title="登录设备" :bordered="false" class="shadow-sm">
      <a-spin :loading="devicesLoading" class="w-full">
        <div v-if="devices.length > 0" class="space-y-3">
          <div
            v-for="device in devices"
            :key="device.device_id"
            class="flex items-center justify-between rounded-lg border border-gray-200 p-4 dark:border-gray-700"
          >
            <div class="flex items-start space-x-3">
              <div class="flex h-10 w-10 items-center justify-center rounded-full bg-blue-100 dark:bg-blue-900">
                <icon-computer class="text-lg text-blue-600 dark:text-blue-300" />
              </div>
              <div>
                <p class="font-medium text-gray-900 dark:text-white">
                  {{ device.device_name }}
                  <a-tag v-if="device.is_current" color="green" size="small" class="ml-2"> 当前设备 </a-tag>
                </p>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ device.location || 'Unknown Location' }} · {{ device.ip }}
                </p>
                <p class="text-xs text-gray-400 dark:text-gray-500">
                  最后活跃: {{ formatDate(device.last_active_at) }}
                </p>
              </div>
            </div>
            <a-button
              v-if="!device.is_current"
              status="danger"
              size="small"
              :loading="removingDeviceId === device.device_id"
              @click="handleRemoveDevice(device.device_id)"
            >
              移除
            </a-button>
          </div>
        </div>
        <a-empty v-else description="暂无登录设备" />
      </a-spin>
    </a-card>

    <!-- 启用 2FA 模态框 -->
    <a-modal v-model:visible="enable2FAModalVisible" title="启用双因素认证" :footer="false" width="500px">
      <div class="space-y-4">
        <a-spin :loading="twoFactorLoading" class="w-full">
          <div v-if="twoFactorData">
            <a-alert type="info" class="mb-4">
              请使用 Google Authenticator 或其他 TOTP 应用扫描二维码，然后输入验证码以完成设置
            </a-alert>

            <!-- QR Code -->
            <div class="flex justify-center py-4">
              <img :src="twoFactorData.qr_code" alt="QR Code" class="h-48 w-48" />
            </div>

            <!-- Secret Key (备用) -->
            <div class="rounded-lg bg-gray-50 p-3 dark:bg-gray-800">
              <p class="text-sm text-gray-600 dark:text-gray-400">或手动输入密钥：</p>
              <p class="font-mono text-sm font-medium text-gray-900 dark:text-white">
                {{ twoFactorData.secret }}
              </p>
            </div>

            <!-- 备用恢复码 -->
            <div class="rounded-lg bg-yellow-50 p-3 dark:bg-yellow-900/20">
              <p class="mb-2 text-sm font-medium text-yellow-800 dark:text-yellow-200">备用恢复码（请妥善保存）：</p>
              <div class="grid grid-cols-2 gap-2">
                <code
                  v-for="code in twoFactorData.backup_codes"
                  :key="code"
                  class="rounded bg-white px-2 py-1 text-xs dark:bg-gray-800"
                >
                  {{ code }}
                </code>
              </div>
            </div>

            <!-- 验证码输入 -->
            <a-form :model="confirm2FAForm" layout="vertical" @submit="handleConfirm2FA">
              <a-form-item label="验证码" field="code" :rules="[{ required: true, message: '请输入 6 位验证码' }]">
                <a-input
                  v-model="confirm2FAForm.code"
                  placeholder="请输入 6 位验证码"
                  maxlength="6"
                  :disabled="twoFactorLoading"
                />
              </a-form-item>
              <a-form-item>
                <a-space>
                  <a-button type="primary" html-type="submit" :loading="twoFactorLoading"> 确认启用 </a-button>
                  <a-button @click="enable2FAModalVisible = false">取消</a-button>
                </a-space>
              </a-form-item>
            </a-form>
          </div>
        </a-spin>
      </div>
    </a-modal>

    <!-- 禁用 2FA 模态框 -->
    <a-modal v-model:visible="disable2FAModalVisible" title="禁用双因素认证" :footer="false" width="400px">
      <a-form :model="disable2FAForm" layout="vertical" @submit="handleDisable2FA">
        <a-alert type="warning" class="mb-4"> 禁用双因素认证会降低您的账号安全性，请谨慎操作 </a-alert>
        <a-form-item label="密码" field="password" :rules="[{ required: true, message: '请输入密码' }]">
          <a-input-password
            v-model="disable2FAForm.password"
            placeholder="请输入您的密码"
            :disabled="twoFactorLoading"
          />
        </a-form-item>
        <a-form-item label="验证码" field="code" :rules="[{ required: true, message: '请输入当前的 6 位验证码' }]">
          <a-input
            v-model="disable2FAForm.code"
            placeholder="请输入当前的 6 位验证码"
            maxlength="6"
            :disabled="twoFactorLoading"
          />
        </a-form-item>
        <a-form-item>
          <a-space>
            <a-button type="primary" status="danger" html-type="submit" :loading="twoFactorLoading">
              确认禁用
            </a-button>
            <a-button @click="disable2FAModalVisible = false">取消</a-button>
          </a-space>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import { IconComputer } from '@arco-design/web-vue/es/icon'
import { useUserStore } from '@paigram/shared-components'
import { securityApi, userApi } from '@/api'
import type { Device, UserSecuritySummary } from '@paigram/shared-components'

// Stores
const userStore = useUserStore()

// 状态
const passwordLoading = ref(false)
const twoFactorLoading = ref(false)
const devicesLoading = ref(false)
const removingDeviceId = ref<string | null>(null)
const twoFactorEnabled = ref(false)
const enable2FAModalVisible = ref(false)
const disable2FAModalVisible = ref(false)

// 表单数据
const passwordForm = ref({
  old_password: '',
  new_password: '',
  confirm_password: '',
})

const confirm2FAForm = ref({
  code: '',
})

const disable2FAForm = ref({
  password: '',
  code: '',
})

const twoFactorData = ref<{
  qr_code: string
  secret: string
  backup_codes: string[]
} | null>(null)

const devices = ref<Device[]>([])
const securitySummary = ref<UserSecuritySummary | null>(null)

// 修改密码
const handleChangePassword = async (): Promise<void> => {
  if (!userStore.userId) {
    Message.error('未找到用户信息')
    return
  }

  passwordLoading.value = true
  try {
    await securityApi.changePassword(userStore.userId, {
      old_password: passwordForm.value.old_password,
      new_password: passwordForm.value.new_password,
    })

    Message.success('密码修改成功')

    // 重置表单
    passwordForm.value = {
      old_password: '',
      new_password: '',
      confirm_password: '',
    }
  } catch (error) {
    console.error('Change password error:', error)
    const err = error as { error?: string; message?: string }
    Message.error(err.error || err.message || '密码修改失败')
  } finally {
    passwordLoading.value = false
  }
}

// 启用 2FA
const handleEnable2FA = async (): Promise<void> => {
  if (!userStore.userId) {
    Message.error('未找到用户信息')
    return
  }

  twoFactorLoading.value = true
  try {
    const response = await securityApi.enable2FA(userStore.userId)
    twoFactorData.value = response.data
    enable2FAModalVisible.value = true
  } catch (error) {
    console.error('Enable 2FA error:', error)
    const err = error as { error?: string; message?: string }
    Message.error(err.error || err.message || '启用双因素认证失败')
  } finally {
    twoFactorLoading.value = false
  }
}

// 确认启用 2FA
const handleConfirm2FA = async (): Promise<void> => {
  if (!userStore.userId || !twoFactorData.value) {
    return
  }

  twoFactorLoading.value = true
  try {
    await securityApi.confirm2FA(userStore.userId, {
      code: confirm2FAForm.value.code,
      secret: twoFactorData.value.secret,
    })

    Message.success('双因素认证已启用')
    twoFactorEnabled.value = true
    enable2FAModalVisible.value = false
    await fetchSecuritySummary()

    // 重置表单
    confirm2FAForm.value.code = ''
    twoFactorData.value = null
  } catch (error) {
    console.error('Confirm 2FA error:', error)
    const err = error as { error?: string; message?: string }
    Message.error(err.error || err.message || '验证码错误，请重试')
  } finally {
    twoFactorLoading.value = false
  }
}

// 显示禁用 2FA 模态框
const showDisable2FAModal = (): void => {
  disable2FAModalVisible.value = true
}

// 禁用 2FA
const handleDisable2FA = async (): Promise<void> => {
  if (!userStore.userId) {
    Message.error('未找到用户信息')
    return
  }

  twoFactorLoading.value = true
  try {
    await securityApi.disable2FA(userStore.userId, {
      password: disable2FAForm.value.password,
      code: disable2FAForm.value.code,
    })

    Message.success('双因素认证已禁用')
    twoFactorEnabled.value = false
    disable2FAModalVisible.value = false
    await fetchSecuritySummary()

    // 重置表单
    disable2FAForm.value = {
      password: '',
      code: '',
    }
  } catch (error) {
    console.error('Disable 2FA error:', error)
    const err = error as { error?: string; message?: string }
    Message.error(err.error || err.message || '禁用失败，请检查密码和验证码')
  } finally {
    twoFactorLoading.value = false
  }
}

// 获取设备列表
const fetchDevices = async (): Promise<void> => {
  if (!userStore.userId) {
    return
  }

  devicesLoading.value = true
  try {
    const response = await securityApi.getDevices(userStore.userId)
    devices.value = response.data.data || []
  } catch (error) {
    console.error('Fetch devices error:', error)
    Message.error('获取设备列表失败')
  } finally {
    devicesLoading.value = false
  }
}

// 获取安全概览
const fetchSecuritySummary = async (): Promise<void> => {
  if (!userStore.userId) return

  try {
    const response = await userApi.getSecuritySummary(userStore.userId)
    securitySummary.value = response.data
    twoFactorEnabled.value = response.data.two_factor_enabled
  } catch (error) {
    console.error('Fetch security summary error:', error)
    Message.error('获取安全概览失败')
  }
}

// 移除设备
const handleRemoveDevice = async (deviceId: string): Promise<void> => {
  if (!userStore.userId) {
    return
  }

  removingDeviceId.value = deviceId
  try {
    await securityApi.removeDevice(userStore.userId, deviceId)
    Message.success('设备已移除')
    await fetchDevices() // 刷新列表
    await fetchSecuritySummary()
  } catch (error) {
    console.error('Remove device error:', error)
    const err = error as { error?: string; message?: string }
    Message.error(err.error || err.message || '移除设备失败')
  } finally {
    removingDeviceId.value = null
  }
}

// 格式化日期
const formatDate = (dateString: string): string => {
  const date = new Date(dateString)
  const now = new Date()
  const diff = now.getTime() - date.getTime()

  // 小于 1 分钟
  if (diff < 60 * 1000) {
    return '刚刚'
  }

  // 小于 1 小时
  if (diff < 60 * 60 * 1000) {
    const minutes = Math.floor(diff / (60 * 1000))
    return `${minutes} 分钟前`
  }

  // 小于 1 天
  if (diff < 24 * 60 * 60 * 1000) {
    const hours = Math.floor(diff / (60 * 60 * 1000))
    return `${hours} 小时前`
  }

  // 小于 7 天
  if (diff < 7 * 24 * 60 * 60 * 1000) {
    const days = Math.floor(diff / (24 * 60 * 60 * 1000))
    return `${days} 天前`
  }

  // 超过 7 天，显示具体日期
  return date.toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const formatAbsoluteDate = (dateString?: string): string => {
  if (!dateString) return '-'
  return new Date(dateString).toLocaleString('zh-CN')
}

// 页面加载时获取数据
onMounted(async () => {
  await Promise.all([fetchDevices(), fetchSecuritySummary()])
})
</script>

<style scoped></style>
