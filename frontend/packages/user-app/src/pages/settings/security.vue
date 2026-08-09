<template>
  <div class="p-6">
    <h1 class="mb-6 text-2xl font-bold">安全设置</h1>
    <a-card title="修改密码" class="mb-4">
      <a-form :model="passwordForm" layout="vertical">
        <a-form-item label="当前密码">
          <a-input-password v-model="passwordForm.current" />
        </a-form-item>
        <a-form-item label="新密码">
          <a-input-password v-model="passwordForm.new" />
        </a-form-item>
        <a-form-item label="确认新密码">
          <a-input-password v-model="passwordForm.confirm" />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" @click="handleChangePassword">修改密码</a-button>
        </a-form-item>
      </a-form>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { reactive } from 'vue'
import { Message } from '@arco-design/web-vue'

const passwordForm = reactive({
  current: '',
  new: '',
  confirm: '',
})

const handleChangePassword = (): void => {
  if (passwordForm.new !== passwordForm.confirm) {
    Message.error('两次输入的密码不一致')
    return
  }
  Message.success('密码修改成功')
}
</script>
