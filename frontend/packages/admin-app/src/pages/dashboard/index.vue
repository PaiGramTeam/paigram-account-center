<template>
  <div class="space-y-6 p-6">
    <div>
      <h1 class="text-2xl font-semibold">管理控制台</h1>
      <p class="mt-1 text-gray-500">从真实账号数据进入日常管理工作。</p>
    </div>

    <a-row :gutter="16">
      <a-col :xs="24" :lg="16">
        <a-card title="最新注册用户">
          <template #extra><a-link @click="router.push('/users/list')">查看全部</a-link></template>
          <a-list :loading="loading" :bordered="false">
            <a-list-item v-for="user in latestUsers" :key="user.id">
              <a-list-item-meta :title="user.display_name" :description="formatDate(user.created_at)">
                <template #avatar>
                  <a-avatar>
                    <img v-if="user.avatar_url" :src="user.avatar_url" alt="用户头像" />
                    <icon-user v-else />
                  </a-avatar>
                </template>
              </a-list-item-meta>
              <template #actions>
                <a-button type="text" @click="router.push(`/users/${user.id}/detail`)">查看</a-button>
              </template>
            </a-list-item>
          </a-list>
          <a-empty v-if="!loading && latestUsers.length === 0" description="暂无用户" />
        </a-card>
      </a-col>
      <a-col :xs="24" :lg="8">
        <a-card title="管理入口">
          <a-space direction="vertical" fill>
            <a-button v-if="hasPermission('user:list')" long @click="router.push('/users/list')">用户管理</a-button>
            <a-button v-if="hasPermission('role:list')" long @click="router.push('/users/roles')">角色与权限</a-button>
            <a-button v-if="hasPermission('platform_account:list')" long @click="router.push('/platform-accounts')">
              平台账号
            </a-button>
            <a-button v-if="hasPermission('audit:list')" long @click="router.push('/system/logs')">审计日志</a-button>
          </a-space>
        </a-card>
        <a-alert
          class="mt-4"
          type="info"
          title="服务健康状态"
          content="部署健康与就绪状态由探针和监控系统提供，本页面不展示未验证的模拟数据。"
        />
      </a-col>
    </a-row>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { IconUser } from '@arco-design/web-vue/es/icon'
import { useUserStore, type UserListItem } from '@paigram/shared-components'
import { userApi } from '@/api'

const router = useRouter()
const userStore = useUserStore()
const loading = ref(false)
const latestUsers = ref<UserListItem[]>([])

const hasPermission = (permission: string): boolean => userStore.hasPermission(permission)
const formatDate = (value: string): string => new Date(value).toLocaleString('zh-CN')

const loadDashboard = async (): Promise<void> => {
  loading.value = true
  try {
    const response = await userApi.getList({ page: 1, page_size: 5 })
    latestUsers.value = response.data
  } finally {
    loading.value = false
  }
}

onMounted(() => void loadDashboard())
</script>
