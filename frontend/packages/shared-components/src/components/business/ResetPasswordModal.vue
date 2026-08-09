<template>
  <a-modal
    v-model:visible="visible"
    title="设置临时密码"
    :mask-closable="false"
    :on-before-ok="handleBeforeOk"
    unmount-on-close
  >
    <a-alert type="warning" class="mb-4">
      将立即替换 {{ userName }} 的当前密码并撤销其他会话，请安全地把临时密码交给用户。
    </a-alert>
    <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
      <a-form-item field="password" label="临时密码">
        <a-input-password v-model="form.password" placeholder="至少 8 位" allow-clear />
      </a-form-item>
      <a-form-item field="confirmation" label="确认临时密码">
        <a-input-password v-model="form.confirmation" placeholder="再次输入临时密码" allow-clear />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import type { FormInstance } from '@arco-design/web-vue'

const props = defineProps<{
  visible: boolean
  userName: string
  resetPassword: (password: string) => Promise<void>
}>()

const emit = defineEmits<{
  'update:visible': [visible: boolean]
}>()

const visible = computed({
  get: () => props.visible,
  set: (value) => emit('update:visible', value),
})
const formRef = ref<FormInstance>()
const form = reactive({ password: '', confirmation: '' })
const rules = {
  password: [
    { required: true, message: '请输入临时密码' },
    { minLength: 8, message: '密码长度至少 8 位' },
  ],
  confirmation: [
    { required: true, message: '请再次输入临时密码' },
    {
      validator: (value: string, callback: (error?: string) => void) =>
        callback(value === form.password ? undefined : '两次输入的密码不一致'),
    },
  ],
}

watch(
  () => props.visible,
  (isVisible) => {
    if (isVisible) return
    form.password = ''
    form.confirmation = ''
    formRef.value?.clearValidate()
  }
)

async function handleBeforeOk(): Promise<boolean> {
  const errors = await formRef.value?.validate()
  if (errors) return false
  await props.resetPassword(form.password)
  return true
}
</script>
