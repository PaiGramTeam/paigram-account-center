<template>
  <div
    class="app-footer flex h-12 items-center justify-center border-t border-gray-200 bg-gray-50 px-6 text-sm text-gray-600 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-400"
  >
    <div>
      <span>{{ copyright }}</span>
      <a-divider direction="vertical" />
      <a href="https://github.com/PaiGramTeam" target="_blank" class="hover:text-blue-500"> GitHub </a>
    </div>
  </div>
</template>

<script setup lang="ts">
interface Props {
  copyright?: string
}

withDefaults(defineProps<Props>(), {
  copyright: `© ${new Date().getFullYear()} PaiGram Team. All rights reserved.`,
})
</script>

<style scoped></style>
