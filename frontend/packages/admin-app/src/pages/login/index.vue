<template>
  <div class="relative flex min-h-screen items-center justify-center overflow-hidden bg-slate-950 px-4 py-10">
    <div
      class="absolute inset-0 bg-[radial-gradient(circle_at_top,_rgba(56,189,248,0.28),_transparent_42%),linear-gradient(135deg,_#020617,_#111827_55%,_#0f172a)]"
    ></div>
    <div class="absolute top-16 -left-16 h-48 w-48 rounded-full bg-cyan-400/10 blur-3xl"></div>
    <div class="absolute -right-8 bottom-10 h-56 w-56 rounded-full bg-sky-500/10 blur-3xl"></div>

    <div
      class="relative w-full max-w-md rounded-3xl border border-white/10 bg-white/95 p-8 shadow-2xl shadow-sky-950/30 backdrop-blur dark:border-gray-700/50 dark:bg-gray-800/95"
    >
      <div class="mb-8 text-center">
        <p class="text-xs font-semibold tracking-[0.35em] text-sky-600 uppercase dark:text-sky-400">Paigram Console</p>
        <h1 class="mt-3 text-3xl font-bold text-slate-900 dark:text-gray-100">管理后台登录</h1>
        <p class="mt-2 text-sm text-slate-500 dark:text-gray-400">使用管理员账号完成验证后进入控制台。</p>
      </div>

      <a-form :model="loginForm" :rules="rules" layout="vertical" @submit-success="handleSubmit">
        <a-form-item field="email" label="邮箱">
          <a-input v-model="loginForm.email" placeholder="请输入邮箱" :disabled="showTwoFactorStep" />
        </a-form-item>

        <a-form-item field="password" label="密码">
          <a-input-password v-model="loginForm.password" placeholder="请输入密码" :disabled="showTwoFactorStep" />
        </a-form-item>

        <a-form-item v-if="showTwoFactorStep" field="totp_code" label="验证码或备用恢复码">
          <AuthTwoFactorStep
            v-model:code="loginForm.totp_code"
            v-model:trust-device="loginForm.trust_device"
            :message="twoFactorMessage"
          />
        </a-form-item>

        <div class="mb-6 flex items-center justify-between text-sm">
          <span class="text-slate-500 dark:text-gray-400">{{
            showTwoFactorStep ? '请完成二级认证后继续' : '支持动态验证码与恢复码'
          }}</span>
          <a-link
            @click="showTwoFactorStep ? resetTwoFactorStep() : undefined"
            :class="showTwoFactorStep ? '' : 'pointer-events-none opacity-0'"
          >
            返回上一步
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

        <a-button type="primary" html-type="submit" long :loading="loading">
          {{ showTwoFactorStep ? '验证并进入控制台' : '登录' }}
        </a-button>
      </a-form>

      <a-divider v-if="!showTwoFactorStep" class="!my-8 !border-slate-200 dark:!border-gray-700">
        <span class="text-xs font-medium tracking-[0.3em] text-slate-400 uppercase dark:text-gray-500">OAuth</span>
      </a-divider>

      <div v-if="!showTwoFactorStep" class="grid grid-cols-2 gap-3">
        <a-button
          v-for="provider in oauthProviders"
          :key="provider.name"
          size="large"
          class="!border-slate-200 !bg-slate-50 !text-slate-700 hover:!border-sky-200 hover:!bg-sky-50 dark:!border-gray-700 dark:!bg-gray-800 dark:!text-gray-300 dark:hover:!border-sky-600 dark:hover:!bg-sky-900/30"
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
</template>

<script setup lang="ts">
import { nextTick, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { IconGithub, IconGoogle } from '@arco-design/web-vue/es/icon'
import { AuthTwoFactorStep, TurnstileWidget, useUserStore } from '@paigram/shared-components'
import { resolveAdminPostLoginRoute, useAuthStore } from '@/stores/auth'
import type { LoginEmailRequest } from '@paigram/shared-components'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const userStore = useUserStore()
const loading = ref(false)
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const captchaToken = ref('')
const turnstileSiteKey = import.meta.env.VITE_TURNSTILE_SITE_KEY?.trim() || ''
const showCaptcha = ref(false)
const showTwoFactorStep = ref(false)
const twoFactorMessage = ref('')

const loginForm = reactive<LoginEmailRequest>({
  email: '',
  password: '',
  totp_code: '',
  trust_device: false,
  captcha_token: undefined,
})

const rules = {
  email: [{ required: true, message: '请输入邮箱' }],
  password: [{ required: true, message: '请输入密码' }],
  totp_code: [{ minLength: 6, message: '验证码长度不能少于6位' }],
}

const oauthProviders = [
  { name: 'google', label: 'Google', icon: IconGoogle },
  { name: 'github', label: 'GitHub', icon: IconGithub },
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

    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
    const fallbackPath = resolveAdminPostLoginRoute(userStore.permissions)
    await router.push(redirect || fallbackPath)
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

const handleOAuthLogin = async (provider: string): Promise<void> => {
  try {
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
    const callbackURL = new URL(`${window.location.origin}/auth/callback/${provider}`)
    if (redirect) {
      callbackURL.searchParams.set('redirect_to', redirect)
    }

    const authURL = await authStore.initiateOAuth(provider, callbackURL.toString())
    window.location.href = authURL
  } catch (_error) {
    // auth store already surfaces a user-facing error
  }
}

function isCaptchaError(error: unknown): boolean {
  const errorCode = (error as { code?: string })?.code
  return errorCode === 'CAPTCHA_REQUIRED' || errorCode === 'CAPTCHA_FAILED'
}

function normalizeTOTPCode(code: string | undefined): string | undefined {
  const normalized = code?.trim().replace(/\s+/g, '')
  return normalized || undefined
}
</script>
