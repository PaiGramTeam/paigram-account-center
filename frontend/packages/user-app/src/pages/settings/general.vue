<template>
  <div class="p-6">
    <h1 class="mb-6 text-2xl font-bold">通用设置</h1>
    <a-form :model="form" layout="vertical">
      <a-form-item label="语言">
        <a-select v-model="form.language">
          <a-option value="zh-CN">简体中文</a-option>
          <a-option value="en-US">English</a-option>
        </a-select>
      </a-form-item>
      <a-form-item label="主题">
        <a-radio-group v-model="form.theme">
          <a-radio value="light">浅色</a-radio>
          <a-radio value="dark">深色</a-radio>
          <a-radio value="auto">跟随系统</a-radio>
        </a-radio-group>
      </a-form-item>
      <a-form-item>
        <a-button type="primary" @click="handleSave">保存设置</a-button>
      </a-form-item>
    </a-form>
  </div>
</template>

<script setup lang="ts">
import { reactive } from 'vue'
import { Message } from '@arco-design/web-vue'

const form = reactive({
  language: 'zh-CN',
  theme: 'light',
})

const handleSave = (): void => {
  Message.success('设置已保存')
}
</script>
