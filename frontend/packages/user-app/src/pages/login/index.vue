<template>
  <div class="flex min-h-screen">
    <div
      class="hidden bg-gradient-to-br from-blue-500 to-indigo-600 lg:flex lg:w-1/2 dark:from-blue-700 dark:to-indigo-800"
    >
      <div class="flex w-full flex-col items-center justify-center px-12 text-white">
        <h1 class="mb-4 text-5xl font-bold">Paigram</h1>
        <p class="max-w-md text-center text-xl">一个账号，畅享所有 PaiGram Bot 系列服务</p>
      </div>
    </div>

    <div class="flex flex-1 items-center justify-center bg-gray-50 px-4 sm:px-6 lg:px-8 dark:bg-gray-900">
      <div class="w-full max-w-md">
        <div class="rounded-lg bg-white px-6 py-8 shadow-lg dark:bg-gray-800">
          <div class="mb-8 text-center">
            <h2 class="text-3xl font-bold text-gray-900 dark:text-gray-100">欢迎回来</h2>
            <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
              没有账号？
              <a-link
                @click="handleGoRegister"
                class="font-medium text-blue-500 hover:text-blue-600 dark:text-blue-400 dark:hover:text-blue-300"
              >
                立即注册
              </a-link>
            </p>
          </div>

          <a-form :model="loginForm" :rules="rules" layout="vertical" @submit-success="handleSubmit">
            <a-form-item field="email" label="邮箱地址">
              <a-input
                v-model="loginForm.email"
                size="large"
                placeholder="请输入邮箱地址"
                allow-clear
                :disabled="showTwoFactorStep"
              >
                <template #prefix>
                  <icon-email />
                </template>
              </a-input>
            </a-form-item>

            <a-form-item field="password" label="密码">
              <a-input-password
                v-model="loginForm.password"
                size="large"
                placeholder="请输入密码"
                allow-clear
                :disabled="showTwoFactorStep"
              >
                <template #prefix>
                  <icon-lock />
                </template>
              </a-input-password>
            </a-form-item>

            <a-form-item v-if="showTwoFactorStep" field="totp_code" label="验证码或备用恢复码">
              <AuthTwoFactorStep
                v-model:code="loginForm.totp_code"
                v-model:trust-device="loginForm.trust_device"
                :message="twoFactorMessage"
              />
            </a-form-item>

            <div class="mb-6 flex items-center justify-between">
              <span class="text-sm text-gray-500 dark:text-gray-400">
                {{ showTwoFactorStep ? '使用验证器或备用恢复码完成本次登录' : '登录后可在账号安全页管理设备与会话' }}
              </span>
              <a-link @click="showTwoFactorStep ? resetTwoFactorStep() : handleForgotPassword()" class="text-sm">
                {{ showTwoFactorStep ? '返回上一步' : '忘记密码？' }}
              </a-link>
            </div>

            <a-form-item v-if="showCaptcha" label="安全验证">
              <TurnstileWidget
                ref="turnstileRef"
                :site-key="turnstileSiteKey"
                action="login"
                @token="handleCaptchaToken"
                @expired="handleCaptchaExpired"
                @error="handleCaptchaError"
              />
            </a-form-item>

            <a-button type="primary" size="large" long html-type="submit" :loading="loading">
              {{ showTwoFactorStep ? '验证并登录' : '登录' }}
            </a-button>
          </a-form>

          <a-divider v-if="!showTwoFactorStep" class="!my-8">
            <span class="text-sm text-gray-500 dark:text-gray-400">或使用其他方式登录</span>
          </a-divider>

          <div v-if="!showTwoFactorStep" class="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <a-button
              v-for="provider in oauthProviders"
              :key="provider.name"
              size="large"
              class="!text-gray-700 dark:!text-gray-300"
              @click="handleOAuthLogin(provider.name)"
            >
              <template #icon>
                <component :is="provider.icon" />
              </template>
              {{ provider.label }}
            </a-button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, reactive, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { IconGithub, IconGoogle, IconSend, IconEmail, IconLock } from '@arco-design/web-vue/es/icon'
import { AuthTwoFactorStep, TurnstileWidget } from '@paigram/shared-components'
import { useAuthStore } from '@/stores/auth'
import type { LoginEmailRequest } from '@paigram/shared-components'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const loginForm = reactive<LoginEmailRequest>({
  email: '',
  password: '',
  totp_code: '',
  trust_device: false,
  captcha_token: undefined,
})

const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const captchaToken = ref('')
const turnstileSiteKey = import.meta.env.VITE_TURNSTILE_SITE_KEY?.trim() || ''
const showCaptcha = ref(false)
const showTwoFactorStep = ref(false)
const twoFactorMessage = ref('')

const loading = ref(false)

const rules = {
  email: [
    { required: true, message: '请输入邮箱地址' },
    { type: 'email', message: '请输入有效的邮箱地址' },
  ],
  password: [
    { required: true, message: '请输入密码' },
    { minLength: 6, message: '密码长度不能少于6位' },
  ],
  totp_code: [{ minLength: 6, message: '验证码长度不能少于6位' }],
}

const oauthProviders = [
  { name: 'google', label: 'Google', icon: IconGoogle },
  { name: 'github', label: 'GitHub', icon: IconGithub },
  { name: 'telegram', label: 'Telegram', icon: IconSend },
]

const handleSubmit = async (values: LoginEmailRequest): Promise<void> => {
  if (showCaptcha.value) {
    if (!turnstileSiteKey) {
      Message.error('当前站点未配置安全验证，请联系管理员')
      return
    }
    if (!captchaToken.value) {
      Message.warning('请先完成安全验证')
      return
    }
  }

  loading.value = true
  try {
    values.captcha_token = showCaptcha.value ? captchaToken.value : undefined

    values.totp_code = normalizeTOTPCode(loginForm.totp_code)
    values.trust_device = showTwoFactorStep.value ? !!loginForm.trust_device : false

    if (showTwoFactorStep.value && !values.totp_code) {
      Message.warning('请输入 2FA 验证码或备用恢复码')
      return
    }

    const result = await authStore.loginWithEmail(values)

    if (result.status === 'requires_totp') {
      showTwoFactorStep.value = true
      twoFactorMessage.value = result.message || ''
      loginForm.totp_code = ''
      loginForm.trust_device = false
      return
    }

    const redirect = route.query.redirect as string
    await router.push(redirect || '/dashboard')
  } catch (error: unknown) {
    console.error('Login failed:', error)
    if (isCaptchaError(error)) {
      showCaptcha.value = true
      captchaToken.value = ''
      await nextTick()
      turnstileRef.value?.reset()
    }
  } finally {
    loading.value = false
  }
}

const resetTwoFactorStep = (): void => {
  showTwoFactorStep.value = false
  twoFactorMessage.value = ''
  loginForm.totp_code = ''
  loginForm.trust_device = false
}

const handleCaptchaToken = (token: string): void => {
  captchaToken.value = token
  loginForm.captcha_token = token
}

const handleCaptchaExpired = (): void => {
  captchaToken.value = ''
  loginForm.captcha_token = undefined
}

const handleCaptchaError = (message: string): void => {
  captchaToken.value = ''
  loginForm.captcha_token = undefined
  Message.warning(message)
}

function isCaptchaError(error: unknown): boolean {
  const errorCode = (error as { code?: string })?.code
  return errorCode === 'CAPTCHA_REQUIRED' || errorCode === 'CAPTCHA_FAILED'
}

function normalizeTOTPCode(code: string | undefined): string | undefined {
  const normalized = code?.trim().replace(/\s+/g, '')
  return normalized || undefined
}

const handleOAuthLogin = async (provider: string): Promise<void> => {
  try {
    const authURL = await authStore.initiateOAuth(provider, `${window.location.origin}/auth/callback/${provider}`)

    window.location.href = authURL
  } catch (_error) {
    // auth store already surfaces a user-facing error
  }
}

const handleGoRegister = (): void => {
  router.push('/register')
}

const handleForgotPassword = (): void => {
  router.push('/forgot-password')
}
</script>
