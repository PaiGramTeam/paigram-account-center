<template>
  <div class="flex min-h-screen flex-col items-center justify-center">
    <a-result status="404" title="404" subtitle="抱歉，您访问的页面不存在">
      <template #extra>
        <a-button type="primary" @click="goHome"> 返回首页 </a-button>
      </template>
    </a-result>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()

const goHome = () => {
  router.push('/')
}
</script>

<style scoped></style>
