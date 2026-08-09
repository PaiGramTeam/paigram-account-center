<template>
  <div class="dashboard-page">
    <div class="mb-6">
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
        欢迎回来，{{ userStore.displayName || '用户' }}
      </h1>
      <p class="mt-2 text-gray-600 dark:text-gray-400">这是您的账号中心控制台，您可以在这里管理您的账号信息</p>
    </div>

    <a-row :gutter="16" class="mb-6">
      <a-col :xs="24" :sm="12" :md="6">
        <a-statistic title="绑定账号" :value="stats.bindingCount" :value-style="{ color: '#165DFF' }">
          <template #suffix>
            <icon-link />
          </template>
        </a-statistic>
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-statistic title="安全评分" :value="stats.securityScore" suffix="%" :value-style="{ color: '#00B42A' }">
          <template #suffix>
            <icon-safe />
          </template>
        </a-statistic>
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-statistic title="登录次数" :value="stats.loginCount" :value-style="{ color: '#F53F3F' }">
          <template #suffix>
            <icon-user />
          </template>
        </a-statistic>
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-statistic title="最后登录" :value-style="{ color: '#FF7D00' }">
          <template #value>
            {{ formatRelativeTime(stats.lastLoginTime) }}
          </template>
          <template #suffix>
            <icon-clock-circle />
          </template>
        </a-statistic>
      </a-col>
    </a-row>

    <a-row :gutter="16">
      <a-col :xs="24" :lg="16">
        <a-card title="账号信息" class="mb-6">
          <div class="flex items-start space-x-6">
            <a-avatar :size="80">
              <template v-if="userStore.avatar">
                <img :src="userStore.avatar" alt="Avatar" />
              </template>
              <template v-else>
                <icon-user />
              </template>
            </a-avatar>
            <div class="flex-1">
              <a-descriptions :column="2" :label-style="{ width: '100px' }">
                <a-descriptions-item label="用户ID">
                  {{ userStore.userId || '-' }}
                </a-descriptions-item>
                <a-descriptions-item label="用户名">
                  {{ userStore.displayName || '-' }}
                </a-descriptions-item>
                <a-descriptions-item label="邮箱">
                  {{ userStore.userInfo?.primary_email || '-' }}
                </a-descriptions-item>
                <a-descriptions-item label="账号状态">
                  <a-tag :color="getStatusColor(userStore.userInfo?.status)">
                    {{ getStatusText(userStore.userInfo?.status) }}
                  </a-tag>
                </a-descriptions-item>
                <a-descriptions-item label="注册时间">
                  {{ formatDate(userStore.userInfo?.created_at) }}
                </a-descriptions-item>
                <a-descriptions-item label="最后更新">
                  {{ formatDate(userStore.userInfo?.updated_at) }}
                </a-descriptions-item>
              </a-descriptions>
            </div>
          </div>
          <a-divider />
          <a-space>
            <a-button type="primary" @click="handleEditProfile">
              <template #icon>
                <icon-edit />
              </template>
              编辑资料
            </a-button>
            <a-button @click="handleChangeAvatar">
              <template #icon>
                <icon-camera />
              </template>
              更换头像
            </a-button>
          </a-space>
        </a-card>
      </a-col>

      <a-col :xs="24" :lg="8">
        <a-card title="快捷操作" class="mb-6">
          <a-row :gutter="[16, 16]">
            <a-col :span="12">
              <a-button type="text" long @click="$router.push('/account/info')">
                <template #icon>
                  <icon-user />
                </template>
                账号信息
              </a-button>
            </a-col>
            <a-col :span="12">
              <a-button type="text" long @click="$router.push('/account/security')">
                <template #icon>
                  <icon-safe />
                </template>
                安全设置
              </a-button>
            </a-col>
            <a-col :span="12">
              <a-button type="text" long @click="$router.push('/account/binding')">
                <template #icon>
                  <icon-link />
                </template>
                账号绑定
              </a-button>
            </a-col>
            <a-col :span="12">
              <a-button type="text" long @click="$router.push('/settings')">
                <template #icon>
                  <icon-settings />
                </template>
                系统设置
              </a-button>
            </a-col>
          </a-row>
        </a-card>

        <a-card title="安全提示">
          <a-space direction="vertical" fill>
            <a-alert
              v-if="!hasSecurePassword"
              type="warning"
              title="密码安全性较低"
              content="建议您设置一个更复杂的密码"
              closable
            />
            <a-alert v-if="!hasTwoFactor" type="info" title="开启双因素认证" content="提高账号安全性" closable />
          </a-space>
        </a-card>
      </a-col>
    </a-row>

    <a-card title="最近活动">
      <a-timeline>
        <a-timeline-item v-for="activity in recentActivities" :key="activity.id">
          <div class="text-sm">
            <div class="font-medium">{{ activity.action }}</div>
            <div class="text-sm text-gray-500 dark:text-gray-400">
              {{ formatDate(activity.time) }} · {{ activity.location }}
            </div>
          </div>
        </a-timeline-item>
      </a-timeline>
      <a-divider />
      <a-button long @click="$router.push('/account/logs')"> 查看所有活动记录 </a-button>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import {
  IconLink,
  IconSafe,
  IconUser,
  IconClockCircle,
  IconEdit,
  IconCamera,
  IconSettings,
} from '@arco-design/web-vue/es/icon'
import { useUserStore } from '@paigram/shared-components'
import { profileApi } from '@/api'

const router = useRouter()
const userStore = useUserStore()

const stats = reactive({
  bindingCount: 3,
  securityScore: 85,
  loginCount: 128,
  lastLoginTime: new Date(),
})

const hasSecurePassword = ref(true)
const hasTwoFactor = ref(false)

const recentActivities = ref([
  { id: 1, action: '登录账号', time: new Date(), location: '北京' },
  { id: 2, action: '修改密码', time: new Date(Date.now() - 172800000), location: '上海' },
  { id: 3, action: '绑定邮箱', time: new Date(Date.now() - 345600000), location: '广州' },
  { id: 4, action: '更新个人信息', time: new Date(Date.now() - 518400000), location: '深圳' },
  { id: 5, action: '开启登录保护', time: new Date(Date.now() - 691200000), location: '杭州' },
])

const getStatusColor = (status?: string): string => {
  const colorMap: Record<string, string> = {
    active: 'green',
    pending: 'blue',
    suspended: 'red',
    deleted: 'gray',
  }
  return colorMap[status || ''] || 'gray'
}

const getStatusText = (status?: string): string => {
  const textMap: Record<string, string> = {
    active: '正常',
    pending: '待处理',
    suspended: '已停用',
    deleted: '已删除',
  }
  return textMap[status || ''] || '未知'
}

const formatDate = (date?: string | Date): string => {
  if (!date) return '-'
  const d = new Date(date)
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const formatRelativeTime = (date: Date): string => {
  const diff = Date.now() - date.getTime()
  const minutes = Math.floor(diff / 60000)
  const hours = Math.floor(diff / 3600000)
  const days = Math.floor(diff / 86400000)

  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes}分钟前`
  if (hours < 24) return `${hours}小时前`
  if (days < 30) return `${days}天前`
  return formatDate(date)
}

const handleEditProfile = (): void => {
  router.push('/profile')
}

const handleChangeAvatar = (): void => {
  Message.info('功能开发中...')
}

const loadUserData = async (): Promise<void> => {
  if (!userStore.userInfo && userStore.userId) {
    try {
      const response = await profileApi.getProfile(userStore.userId)
      userStore.setUserInfo({
        id: response.data.user_id,
        display_name: response.data.display_name,
        primary_email: response.data.primary_email,
        avatar_url: response.data.avatar_url,
        status: 'active',
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        roles: [],
        permissions: [],
      })
    } catch (error) {
      console.error('Failed to load user profile:', error)
    }
  }
}

onMounted(() => {
  if (!userStore.isLogin) {
    router.push('/login')
    return
  }

  loadUserData()
})
</script>
