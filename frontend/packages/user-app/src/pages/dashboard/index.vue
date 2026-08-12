<template>
  <div class="space-y-6 p-6">
    <div>
      <h1 class="text-2xl font-semibold">欢迎回来，{{ userStore.displayName || '用户' }}</h1>
      <p class="mt-2 text-gray-500">查看真实的平台账号、角色、授权与安全状态。</p>
    </div>

    <a-spin :loading="loading" class="w-full">
      <a-row :gutter="16">
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="平台账号" :value="summary.total_bindings" />
        </a-col>
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="有效账号" :value="summary.active_bindings" />
        </a-col>
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="游戏角色" :value="summary.total_profiles" />
        </a-col>
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="已启用应用" :value="summary.enabled_consumers" />
        </a-col>
      </a-row>

      <a-alert
        v-if="summary.invalid_bindings + summary.refresh_required_bindings > 0"
        class="mt-4"
        type="warning"
        title="部分平台账号需要处理"
        :content="`${summary.invalid_bindings} 个凭据无效，${summary.refresh_required_bindings} 个需要刷新。`"
      >
        <template #action
          ><a-button size="small" @click="router.push('/platform-accounts')">立即处理</a-button></template
        >
      </a-alert>
    </a-spin>

    <a-row :gutter="16">
      <a-col :xs="24" :lg="16">
        <a-card title="账号信息">
          <div class="flex items-start gap-6">
            <a-avatar :size="72">
              <img v-if="userStore.avatar" :src="userStore.avatar" alt="用户头像" />
              <icon-user v-else />
            </a-avatar>
            <a-descriptions class="flex-1" :column="2">
              <a-descriptions-item label="用户 ID">{{ userStore.userId || '-' }}</a-descriptions-item>
              <a-descriptions-item label="显示名称">{{ userStore.displayName || '-' }}</a-descriptions-item>
              <a-descriptions-item label="邮箱">{{ userStore.userInfo?.primary_email || '-' }}</a-descriptions-item>
              <a-descriptions-item label="最后登录">{{ formatDate(security.last_login_at) }}</a-descriptions-item>
              <a-descriptions-item label="活动会话">{{ security.active_session_count }}</a-descriptions-item>
              <a-descriptions-item label="已知设备">{{ security.device_count }}</a-descriptions-item>
            </a-descriptions>
          </div>
          <a-divider />
          <a-space>
            <a-button type="primary" @click="router.push('/account/info')">编辑资料</a-button>
            <a-button @click="router.push('/account/security')">安全设置</a-button>
          </a-space>
        </a-card>
      </a-col>
      <a-col :xs="24" :lg="8">
        <a-card title="快捷操作">
          <a-space direction="vertical" fill>
            <a-button long @click="router.push('/platform-accounts')">管理平台账号</a-button>
            <a-button long @click="router.push('/apps')">管理应用授权</a-button>
            <a-button long @click="router.push('/account/binding')">管理登录方式</a-button>
          </a-space>
        </a-card>
      </a-col>
    </a-row>

    <a-card title="最近活动">
      <a-timeline v-if="activities.length > 0">
        <a-timeline-item v-for="activity in activities" :key="activity.id">
          <div class="font-medium">{{ activity.action }}</div>
          <div class="text-sm text-gray-500">
            {{ formatDate(activity.created_at) }}<span v-if="activity.ip"> · {{ activity.ip }}</span>
          </div>
        </a-timeline-item>
      </a-timeline>
      <a-empty v-else description="暂无活动记录" />
      <a-button class="mt-3" long @click="router.push('/account/logs')">查看所有活动记录</a-button>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { IconUser } from '@arco-design/web-vue/es/icon'
import {
  useUserStore,
  type ActivityLogItem,
  type DashboardSummary,
  type SecurityOverview,
} from '@paigram/shared-components'
import { platformAccountsApi, profileApi, securityApi } from '@/api'

const router = useRouter()
const userStore = useUserStore()
const loading = ref(false)
const activities = ref<ActivityLogItem[]>([])
const summary = reactive<DashboardSummary>({
  total_bindings: 0,
  active_bindings: 0,
  invalid_bindings: 0,
  refresh_required_bindings: 0,
  total_profiles: 0,
  enabled_consumers: 0,
})
const security = reactive<SecurityOverview>({
  active_session_count: 0,
  device_count: 0,
  failed_logins_last_30_days: 0,
  two_factor_enabled: false,
  user_id: 0,
})

const formatDate = (value?: string): string => (value ? new Date(value).toLocaleString('zh-CN') : '-')

const loadDashboard = async (): Promise<void> => {
  loading.value = true
  try {
    const [summaryResponse, securityResponse, activityResponse] = await Promise.all([
      platformAccountsApi.getDashboardSummary(),
      securityApi.getOverview(),
      profileApi.getActivityLogs({ page: 1, page_size: 5 }),
    ])
    Object.assign(summary, summaryResponse.data)
    Object.assign(security, securityResponse.data)
    activities.value = activityResponse.data.data.data
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (!userStore.isLogin) {
    void router.push('/login')
    return
  }
  void loadDashboard()
})
</script>
