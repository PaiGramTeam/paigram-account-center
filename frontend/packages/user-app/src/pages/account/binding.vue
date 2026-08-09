<template>
  <div>
    <a-card title="账号绑定" :loading="loading">
      <template #extra>
        <a-button type="primary" @click="handleBindAccount">
          <template #icon>
            <icon-plus />
          </template>
          绑定新账号
        </a-button>
      </template>

      <div v-if="boundAccounts.length === 0" class="py-12 text-center">
        <icon-empty class="text-6xl text-gray-300" />
        <p class="mt-4 text-gray-500">暂无绑定的第三方账号</p>
        <p class="mt-2 text-sm text-gray-400">绑定第三方账号后，可以使用多种方式登录</p>
      </div>

      <a-list v-else :data="boundAccounts" :bordered="false">
        <template #item="{ item }">
          <a-list-item>
            <div class="flex w-full items-center justify-between">
              <div class="flex items-center space-x-4">
                <!-- Provider Icon -->
                <div class="flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-gray-800">
                  <component :is="getProviderIcon(item.provider)" class="text-2xl" />
                </div>

                <!-- Account Info -->
                <div>
                  <div class="flex items-center space-x-2">
                    <span class="font-medium">{{ getProviderName(item.provider) }}</span>
                    <a-tag v-if="item.is_primary" color="blue" size="small">主账号</a-tag>
                  </div>
                  <div class="mt-1 text-sm text-gray-500">
                    <span v-if="item.display_name">{{ item.display_name }}</span>
                    <span v-else-if="item.email">{{ item.email }}</span>
                    <span v-else>账号ID: {{ item.provider_account_id }}</span>
                  </div>
                  <div class="mt-1 text-xs text-gray-400">
                    绑定于 {{ formatDate(item.bound_at) }}
                    <span v-if="item.last_used_at"> · 最后使用 {{ formatDate(item.last_used_at) }}</span>
                  </div>
                </div>
              </div>

              <!-- Actions -->
              <div>
                <a-popconfirm
                  content="确定要解绑此账号吗？解绑后将无法使用该方式登录。"
                  type="warning"
                  @ok="handleUnbind(item)"
                >
                  <a-button
                    type="text"
                    status="danger"
                    :disabled="boundAccounts.length === 1"
                    :loading="unbindingProvider === item.provider"
                  >
                    解绑
                  </a-button>
                </a-popconfirm>
                <a-tooltip v-if="boundAccounts.length === 1" content="至少需要保留一种登录方式">
                  <icon-info-circle class="ml-2 text-gray-400" />
                </a-tooltip>
              </div>
            </div>
          </a-list-item>
        </template>
      </a-list>

      <!-- 分页 -->
      <div v-if="pagination.total > pagination.page_size" class="mt-4 flex justify-end">
        <a-pagination
          v-model:current="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          show-total
          @change="loadBoundAccounts"
        />
      </div>
    </a-card>

    <!-- 绑定账号弹窗 -->
    <a-modal
      v-model:visible="bindModalVisible"
      title="绑定新账号"
      :footer="false"
      :width="500"
      @cancel="bindModalVisible = false"
    >
      <div class="space-y-4">
        <div class="text-sm text-gray-600">选择要绑定的第三方平台：</div>

        <!-- Telegram -->
        <a-button v-if="!isProviderBound('telegram')" class="w-full" size="large" @click="handleBindTelegram">
          <template #icon>
            <icon-telegram class="text-xl" />
          </template>
          <span class="ml-2">绑定 Telegram</span>
        </a-button>

        <!-- Google -->
        <a-button v-if="!isProviderBound('google')" class="w-full" size="large" @click="handleBindOAuth('google')">
          <template #icon>
            <icon-google class="text-xl" />
          </template>
          <span class="ml-2">绑定 Google</span>
        </a-button>

        <!-- GitHub -->
        <a-button v-if="!isProviderBound('github')" class="w-full" size="large" @click="handleBindOAuth('github')">
          <template #icon>
            <icon-github class="text-xl" />
          </template>
          <span class="ml-2">绑定 GitHub</span>
        </a-button>

        <div v-if="allProvidersBound" class="py-8 text-center text-gray-500">
          <icon-check-circle class="mb-2 text-4xl text-green-500" />
          <p>所有支持的第三方账号都已绑定</p>
        </div>
      </div>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import { IconPlus, IconEmpty, IconInfoCircle, IconCheckCircle } from '@arco-design/web-vue/es/icon'
import { useUserStore } from '@paigram/shared-components'
import { profileApi } from '@/api'
import type { BoundAccount } from '@paigram/shared-components'

// 自定义 Provider 图标组件
const IconTelegram = { template: '<span>📱</span>' }
const IconGoogle = { template: '<span>🔍</span>' }
const IconGithub = { template: '<span>🐙</span>' }

const userStore = useUserStore()
const loading = ref(false)
const boundAccounts = ref<BoundAccount[]>([])
const bindModalVisible = ref(false)
const unbindingProvider = ref<string | null>(null)

const pagination = ref({
  page: 1,
  page_size: 20,
  total: 0,
  total_pages: 0,
})

// 获取 Provider 图标
const getProviderIcon = (provider: string) => {
  const iconMap: Record<string, unknown> = {
    telegram: IconTelegram,
    google: IconGoogle,
    github: IconGithub,
  }
  return iconMap[provider] || IconEmpty
}

// 获取 Provider 名称
const getProviderName = (provider: string): string => {
  const nameMap: Record<string, string> = {
    telegram: 'Telegram',
    google: 'Google',
    github: 'GitHub',
  }
  return nameMap[provider] || provider
}

// 格式化日期
const formatDate = (date?: string): string => {
  if (!date) return '-'
  return new Date(date).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// 检查 Provider 是否已绑定
const isProviderBound = (provider: string): boolean => {
  return boundAccounts.value.some((account) => account.provider === provider)
}

// 所有支持的 Provider 是否都已绑定
const allProvidersBound = computed(() => {
  const supportedProviders = ['telegram', 'google', 'github']
  return supportedProviders.every((provider) => isProviderBound(provider))
})

// 加载已绑定的账号列表
const loadBoundAccounts = async (): Promise<void> => {
  const userId = userStore.userInfo?.id
  if (!userId) {
    Message.error('未获取到用户信息')
    return
  }

  loading.value = true
  try {
    const response = await profileApi.getBoundAccounts(userId)
    boundAccounts.value = response.data.data || []
    pagination.value = response.data.pagination
  } catch (error) {
    console.error('加载绑定账号失败:', error)
    Message.error('加载绑定账号失败')
  } finally {
    loading.value = false
  }
}

// 打开绑定账号弹窗
const handleBindAccount = (): void => {
  bindModalVisible.value = true
}

// 绑定 Telegram
const handleBindTelegram = (): void => {
  Message.info('Telegram 绑定功能开发中，请使用 Telegram Login Widget')
  // TODO: 实现 Telegram 绑定逻辑
  // 1. 显示 Telegram Login Widget
  // 2. 获取 Telegram auth data
  // 3. 调用 profileApi.bindAccount()
}

// 绑定 OAuth（Google/GitHub）
const handleBindOAuth = async (provider: string): Promise<void> => {
  Message.info(`${getProviderName(provider)} 绑定功能开发中`)
  // TODO: 实现 OAuth 绑定逻辑
  // 1. 调用 authApi.initiateOAuth() 获取授权 URL
  // 2. 跳转到 OAuth 授权页面
  // 3. 在回调页面处理绑定结果
}

// 解绑账号
const handleUnbind = async (account: BoundAccount): Promise<void> => {
  const userId = userStore.userInfo?.id
  if (!userId) {
    Message.error('未获取到用户信息')
    return
  }

  // 检查是否至少保留一个绑定账号
  if (boundAccounts.value.length <= 1) {
    Message.warning('至少需要保留一种登录方式')
    return
  }

  unbindingProvider.value = account.provider
  try {
    await profileApi.unbindAccount(userId, account.provider)
    Message.success(`已解绑 ${getProviderName(account.provider)}`)

    // 重新加载列表
    await loadBoundAccounts()
  } catch (error) {
    console.error('解绑账号失败:', error)
    const errorMessage = error instanceof Error ? error.message : '解绑失败，请稍后重试'
    Message.error(errorMessage)
  } finally {
    unbindingProvider.value = null
  }
}

// 组件挂载时加载数据
onMounted(() => {
  loadBoundAccounts()
})
</script>
