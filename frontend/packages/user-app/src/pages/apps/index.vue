<template>
  <div class="space-y-6 p-6">
    <div>
      <h1 class="text-2xl font-semibold">应用授权</h1>
      <p class="mt-1 text-gray-500">查看并撤销 Bot 对平台账号的访问，以及已关联的 Bot 身份。</p>
    </div>

    <a-card title="消费者授权">
      <a-spin :loading="loading" class="w-full">
        <div
          v-for="item in grants"
          :key="`${item.binding.id}:${item.grant.consumer}`"
          class="mb-3 rounded border border-gray-200 p-4"
        >
          <div class="flex items-center justify-between gap-4">
            <div>
              <div class="font-medium">{{ item.grant.consumer }}</div>
              <div class="mt-1 text-sm text-gray-500">
                {{ item.binding.display_name }} · {{ item.binding.platform }}
              </div>
              <div class="mt-1 text-xs text-gray-500">{{ item.grant.actions?.join('、') || '未配置动作' }}</div>
            </div>
            <a-popconfirm content="撤销后，该应用签发的旧票据将失效。确认继续？" @ok="revokeGrant(item)">
              <a-button status="danger">撤销授权</a-button>
            </a-popconfirm>
          </div>
        </div>
        <a-empty v-if="grants.length === 0" description="暂无启用的应用授权" />
      </a-spin>
    </a-card>

    <a-card title="Bot 身份关联">
      <a-list :data="identities" :bordered="false">
        <template #item="{ item }">
          <a-list-item>
            <a-list-item-meta
              :title="item.external_username || item.external_user_id"
              :description="`${item.bot_id} · 关联于 ${formatDate(item.linked_at)}`"
            />
            <template #actions>
              <a-popconfirm content="确认解除该 Bot 身份关联？" @ok="removeIdentity(item.bot_id)">
                <a-button type="text" status="danger">解除关联</a-button>
              </a-popconfirm>
            </template>
          </a-list-item>
        </template>
      </a-list>
      <a-empty v-if="identities.length === 0" description="暂无 Bot 身份关联" />
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { platformAccountsApi } from '@/api'
import type { BotIdentity, ConsumerGrant, PlatformBinding } from '@paigram/shared-components'

interface GrantItem {
  binding: PlatformBinding
  grant: ConsumerGrant
}

const loading = ref(false)
const grants = ref<GrantItem[]>([])
const identities = ref<BotIdentity[]>([])

const loadData = async (): Promise<void> => {
  loading.value = true
  try {
    const [bindingResponse, identityResponse] = await Promise.all([
      platformAccountsApi.list({ page: 1, page_size: 100 }),
      platformAccountsApi.listBotIdentities(),
    ])
    const bindings = bindingResponse.data.items ?? []
    const grantResponses = await Promise.all(bindings.map((binding) => platformAccountsApi.listGrants(binding.id)))
    grants.value = grantResponses.flatMap((response, index) =>
      (response.data.items ?? [])
        .filter((grant) => grant.status === 'active')
        .map((grant) => ({ binding: bindings[index]!, grant }))
    )
    identities.value = identityResponse.data ?? []
  } finally {
    loading.value = false
  }
}

const revokeGrant = async (item: GrantItem): Promise<void> => {
  await platformAccountsApi.changeGrant(item.binding.id, item.grant.consumer, { enabled: false })
  Message.success('应用授权已撤销')
  await loadData()
}

const removeIdentity = async (botId: string): Promise<void> => {
  await platformAccountsApi.removeBotIdentity(botId)
  Message.success('Bot 身份关联已解除')
  await loadData()
}

const formatDate = (value: string): string => new Date(value).toLocaleString('zh-CN')

onMounted(() => void loadData())
</script>
