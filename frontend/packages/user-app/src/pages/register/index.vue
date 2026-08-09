<template>
  <div class="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-900">
    <div class="w-full max-w-md rounded-lg bg-white p-8 shadow-md dark:bg-gray-800">
      <h1 class="mb-6 text-center text-2xl font-bold dark:text-gray-100">注册账号</h1>
      <a-form ref="formRef" :model="form" :rules="rules" layout="vertical" @submit="handleSubmit">
        <a-form-item field="display_name" label="显示名称">
          <a-input v-model="form.display_name" placeholder="请输入显示名称" allow-clear />
        </a-form-item>
        <a-form-item field="email" label="邮箱地址">
          <a-input v-model="form.email" placeholder="请输入邮箱地址" type="email" allow-clear />
        </a-form-item>
        <a-form-item field="password" label="密码">
          <a-input-password v-model="form.password" placeholder="请输入密码（8-72个字符）" allow-clear />
          <template #extra>
            <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              密码长度为8-72个字符，建议包含大小写字母、数字和特殊字符
            </div>
          </template>
        </a-form-item>
        <a-form-item field="confirmPassword" label="确认密码">
          <a-input-password v-model="form.confirmPassword" placeholder="请再次输入密码" allow-clear />
        </a-form-item>
        <a-form-item field="locale" label="语言偏好（可选）">
          <a-select v-model="form.locale" placeholder="选择语言偏好" allow-clear>
            <a-option value="zh_CN">简体中文</a-option>
            <a-option value="en_US">English</a-option>
            <a-option value="ja_JP">日本語</a-option>
          </a-select>
        </a-form-item>
        <a-form-item v-if="turnstileEnabled" label="安全验证">
          <TurnstileWidget
            ref="turnstileRef"
            :site-key="turnstileSiteKey"
            action="register"
            @token="handleCaptchaToken"
            @expired="handleCaptchaExpired"
            @error="handleCaptchaError"
          />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" html-type="submit" long :loading="loading">注册</a-button>
        </a-form-item>
      </a-form>
      <div class="mt-4 text-center text-sm text-gray-600 dark:text-gray-400">
        已有账号？<router-link to="/login" class="text-blue-500 hover:underline dark:text-blue-400"
          >立即登录</router-link
        >
      </div>

      <!-- 注册成功提示 -->
      <a-modal
        v-model:visible="successModalVisible"
        title="注册成功"
        :footer="false"
        :closable="false"
        :mask-closable="false"
      >
        <div class="text-center">
          <div class="mb-4">
            <icon-check-circle-fill class="text-6xl text-green-500" />
          </div>
          <h3 class="mb-2 text-lg font-semibold dark:text-gray-100">注册成功！</h3>
          <p v-if="requiresEmailVerification" class="mb-4 text-gray-600 dark:text-gray-300">
            我们已向 <span class="font-medium">{{ form.email }}</span> 发送了验证邮件，请查收并完成邮箱验证。
          </p>
          <p v-else class="mb-4 text-gray-600 dark:text-gray-300">您的账号已创建成功，现在可以登录了。</p>
          <a-button type="primary" @click="goToLogin">前往登录</a-button>
        </div>
      </a-modal>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { IconCheckCircleFill } from '@arco-design/web-vue/es/icon'
import type { FormInstance } from '@arco-design/web-vue'
import { authApi } from '@/api'
import { TurnstileWidget } from '@paigram/shared-components'
import type { RegisterEmailRequest } from '@paigram/shared-components'

const router = useRouter()
const loading = ref(false)
const formRef = ref<FormInstance>()
const successModalVisible = ref(false)
const requiresEmailVerification = ref(false)
const captchaToken = ref('')
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const turnstileSiteKey = import.meta.env.VITE_TURNSTILE_SITE_KEY?.trim() || ''
const turnstileEnabled = Boolean(turnstileSiteKey)

const form = reactive<RegisterEmailRequest & { confirmPassword: string }>({
  display_name: '',
  email: '',
  password: '',
  confirmPassword: '',
  locale: 'zh_CN',
})

// 表单验证规则
const rules = {
  display_name: [
    { required: true, message: '请输入显示名称' },
    { minLength: 1, message: '显示名称不能为空' },
    { maxLength: 50, message: '显示名称不能超过50个字符' },
  ],
  email: [
    { required: true, message: '请输入邮箱地址' },
    {
      type: 'email' as const,
      message: '请输入有效的邮箱地址',
    },
  ],
  password: [
    { required: true, message: '请输入密码' },
    { minLength: 8, message: '密码长度至少为8个字符' },
    { maxLength: 72, message: '密码长度不能超过72个字符' },
  ],
  confirmPassword: [
    { required: true, message: '请再次输入密码' },
    {
      validator: (value: string, callback: (error?: string) => void) => {
        if (value !== form.password) {
          callback('两次输入的密码不一致')
        } else {
          callback()
        }
      },
    },
  ],
}

const handleSubmit = async (): Promise<void> => {
  const valid = await formRef.value?.validate()
  if (!valid) return

  if (turnstileEnabled && !captchaToken.value) {
    Message.warning('请先完成安全验证')
    return
  }

  loading.value = true
  try {
    const registerData: RegisterEmailRequest = {
      display_name: form.display_name,
      email: form.email,
      password: form.password,
      locale: form.locale,
      captcha_token: captchaToken.value || undefined,
    }

    const response = await authApi.register(registerData)

    // 检查是否需要邮箱验证
    requiresEmailVerification.value = response.data.requires_email_verification

    // 显示成功提示
    successModalVisible.value = true
  } catch (error: unknown) {
    captchaToken.value = ''
    turnstileRef.value?.reset()
    console.error('注册失败:', error)
    const errorMessage = error instanceof Error ? error.message : '注册失败，请稍后重试'
    Message.error(errorMessage)
  } finally {
    loading.value = false
  }
}

const handleCaptchaToken = (token: string): void => {
  captchaToken.value = token
}

const handleCaptchaExpired = (): void => {
  captchaToken.value = ''
}

const handleCaptchaError = (message: string): void => {
  captchaToken.value = ''
  Message.warning(message)
}

const goToLogin = (): void => {
  successModalVisible.value = false
  router.push('/login')
}
</script>
