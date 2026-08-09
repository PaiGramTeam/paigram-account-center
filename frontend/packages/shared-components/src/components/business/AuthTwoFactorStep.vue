<template>
  <div class="space-y-4">
    <div class="rounded-2xl border border-blue-100 bg-blue-50 px-4 py-3 text-sm text-blue-700">
      <p class="font-medium text-blue-900">需要二级认证</p>
      <p class="mt-1">{{ message || defaultMessage }}</p>
    </div>

    <a-input
      :model-value="code"
      size="large"
      placeholder="请输入 2FA 验证码"
      allow-clear
      maxlength="8"
      @update:model-value="handleCodeUpdate"
    >
      <template #prefix>
        <icon-safe />
      </template>
    </a-input>

    <div class="rounded-xl bg-gray-50 px-4 py-3 text-xs leading-6 text-gray-600 dark:bg-gray-800 dark:text-gray-400">
      你可以输入身份验证器中的动态验证码，也可以输入一次性备用恢复码。
    </div>

    <a-checkbox :model-value="trustDevice" @update:model-value="handleTrustDeviceUpdate">信任此设备 30 天</a-checkbox>
  </div>
</template>

<script setup lang="ts">
import { IconSafe } from '@arco-design/web-vue/es/icon'

interface Props {
  code?: string
  trustDevice?: boolean
  message?: string
}

withDefaults(defineProps<Props>(), {
  code: '',
  trustDevice: false,
  message: '',
})

const emit = defineEmits<{
  'update:code': [value: string]
  'update:trustDevice': [value: boolean]
}>()

const defaultMessage = '请输入身份验证器中的 6 位验证码，或使用备用恢复码继续登录。'

const handleCodeUpdate = (value: string | number | boolean | undefined): void => {
  emit('update:code', typeof value === 'string' ? value : '')
}

const handleTrustDeviceUpdate = (value: string | number | boolean | undefined): void => {
  emit('update:trustDevice', value === true)
}
</script>
