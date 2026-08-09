<template>
  <div class="flex min-h-screen flex-col items-center justify-center">
    <h1 class="text-6xl font-bold text-gray-400">403</h1>
    <p class="mt-4 text-xl text-gray-600">无权访问</p>
    <a-button type="primary" class="mt-6" @click="$router.push('/')">返回首页</a-button>
  </div>
</template>

<script setup lang="ts"></script>
