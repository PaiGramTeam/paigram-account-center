<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-semibold">服务注册</h1>
      <p class="mt-1 text-gray-500">管理平台适配器，以及调用账号中心的 Bot 服务凭据。</p>
    </div>

    <a-card :bordered="false">
      <a-tabs v-model:active-key="activeTab">
        <a-tab-pane v-if="hasPermission('platform:list')" key="platforms" title="平台适配器">
          <PlatformServicesTab />
        </a-tab-pane>
        <a-tab-pane v-if="hasPermission('bot:list')" key="credentials" title="服务凭据">
          <ServiceCredentialsTab />
        </a-tab-pane>
      </a-tabs>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useUserStore } from '@paigram/shared-components'
import PlatformServicesTab from './PlatformServicesTab.vue'
import ServiceCredentialsTab from './ServiceCredentialsTab.vue'

const userStore = useUserStore()
const hasPermission = (permission: string): boolean => userStore.hasPermission(permission)
const activeTab = ref(hasPermission('platform:list') ? 'platforms' : 'credentials')
</script>
