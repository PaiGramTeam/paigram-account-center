<template>
  <div class="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-900">
    <div class="w-full max-w-md rounded-lg bg-white p-8 shadow-md dark:bg-gray-800">
      <h1 class="mb-6 text-center text-2xl font-bold dark:text-gray-100">重置密码</h1>
      <a-alert v-if="!token" type="error" class="mb-4">重置链接无效或缺少令牌，请重新申请。</a-alert>
      <a-form :model="form" :rules="rules" layout="vertical" @submit-success="handleSubmit">
        <a-form-item field="password" label="新密码">
          <a-input-password v-model="form.password" placeholder="请输入新密码（8-72 个字符）" />
        </a-form-item>
        <a-form-item field="confirmation" label="确认新密码">
          <a-input-password v-model="form.confirmation" placeholder="请再次输入新密码" />
        </a-form-item>
        <a-button type="primary" html-type="submit" long :loading="loading" :disabled="!token">重置密码</a-button>
      </a-form>
      <div class="mt-4 text-center text-sm">
        <router-link to="/login" class="text-blue-500 dark:text-blue-400">返回登录</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { authApi } from '@/api'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))
const form = reactive({ password: '', confirmation: '' })
const rules = {
  password: [
    { required: true, message: '请输入新密码' },
    { minLength: 8, message: '密码长度至少为 8 个字符' },
    { maxLength: 72, message: '密码长度不能超过 72 个字符' },
  ],
  confirmation: [
    { required: true, message: '请再次输入新密码' },
    {
      validator: (value: string, callback: (error?: string) => void) =>
        callback(value === form.password ? undefined : '两次输入的密码不一致'),
    },
  ],
}

const handleSubmit = async (): Promise<void> => {
  if (!token.value) return
  loading.value = true
  try {
    await authApi.resetPassword({ token: token.value, new_password: form.password })
    Message.success('密码已重置，请使用新密码登录')
    await router.replace('/login')
  } catch (_error) {
    Message.error('重置失败，链接可能已失效')
  } finally {
    loading.value = false
  }
}
</script>
