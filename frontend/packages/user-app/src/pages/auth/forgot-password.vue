<template>
  <div class="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-900">
    <div class="w-full max-w-md rounded-lg bg-white p-8 shadow-md dark:bg-gray-800">
      <h1 class="mb-6 text-center text-2xl font-bold dark:text-gray-100">找回密码</h1>
      <a-form :model="form" :rules="rules" layout="vertical" @submit-success="handleSubmit">
        <a-form-item field="email" label="邮箱">
          <a-input v-model="form.email" placeholder="请输入注册邮箱" />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" html-type="submit" long :loading="loading">发送重置链接</a-button>
        </a-form-item>
      </a-form>
      <div class="mt-4 text-center text-sm text-gray-600 dark:text-gray-400">
        <router-link to="/login" class="text-blue-500 dark:text-blue-400">返回登录</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { Message } from '@arco-design/web-vue'
import { authApi } from '@/api'

const loading = ref(false)
const form = reactive({ email: '' })
const rules = {
  email: [
    { required: true, message: '请输入注册邮箱' },
    { type: 'email' as const, message: '请输入有效的邮箱地址' },
  ],
}

const handleSubmit = async (): Promise<void> => {
  loading.value = true
  try {
    await authApi.forgotPassword({ email: form.email })
    Message.success('如果该邮箱已注册，重置链接会发送到您的邮箱')
  } catch (_error) {
    Message.error('发送失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>
