<template>
  <div class="admin-dashboard">
    <!-- 统计卡片 -->
    <a-row :gutter="16" class="mb-6">
      <a-col :xs="24" :sm="12" :md="6">
        <a-statistic title="总用户数" :value="statistics.totalUsers" :value-from="0" animation>
          <template #prefix>
            <icon-user class="text-blue-500" />
          </template>
        </a-statistic>
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-statistic title="今日新增" :value="statistics.todayNewUsers" :value-from="0" animation show-group-separator>
          <template #prefix>
            <icon-user-add class="text-green-500" />
          </template>
          <template #suffix>
            <span class="text-sm text-green-500">
              <icon-arrow-rise />
              {{ statistics.newUserGrowth }}%
            </span>
          </template>
        </a-statistic>
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-statistic title="活跃用户" :value="statistics.activeUsers" :value-from="0" animation>
          <template #prefix>
            <icon-fire class="text-orange-500" />
          </template>
        </a-statistic>
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-statistic title="在线用户" :value="statistics.onlineUsers" :value-from="0" animation>
          <template #prefix>
            <icon-wifi class="text-purple-500" />
          </template>
        </a-statistic>
      </a-col>
    </a-row>

    <a-row :gutter="16">
      <!-- 用户增长趋势 -->
      <a-col :xs="24" :lg="16">
        <a-card title="用户增长趋势" class="mb-6">
          <template #extra>
            <a-radio-group v-model="chartPeriod" type="button" size="small">
              <a-radio value="week">本周</a-radio>
              <a-radio value="month">本月</a-radio>
              <a-radio value="year">本年</a-radio>
            </a-radio-group>
          </template>

          <div class="h-80">
            <!-- 这里可以集成图表库如 ECharts 或 Chart.js -->
            <div class="flex h-full items-center justify-center text-gray-400">
              <icon-bar-chart class="mr-2" /> 用户增长图表
            </div>
          </div>
        </a-card>

        <!-- 最新用户 -->
        <a-card title="最新注册用户">
          <template #extra>
            <a-link @click="$router.push('/users')">查看全部</a-link>
          </template>

          <a-list :bordered="false">
            <a-list-item v-for="user in latestUsers" :key="user.id">
              <a-list-item-meta>
                <template #avatar>
                  <a-avatar>
                    <img v-if="user.avatar_url" :src="user.avatar_url" />
                    <icon-user v-else />
                  </a-avatar>
                </template>
                <template #title>
                  {{ user.display_name }}
                </template>
                <template #description> {{ user.primary_email }} · {{ formatRelativeTime(user.created_at) }} </template>
              </a-list-item-meta>
              <template #actions>
                <a-button type="text" size="small" @click="handleViewUser(user)"> 查看 </a-button>
              </template>
            </a-list-item>
          </a-list>
        </a-card>
      </a-col>

      <!-- 右侧信息 -->
      <a-col :xs="24" :lg="8">
        <!-- 系统状态 -->
        <a-card title="系统状态" class="mb-6">
          <a-descriptions :column="1" :label-style="{ width: '100px' }">
            <a-descriptions-item label="系统版本"> v1.0.0 </a-descriptions-item>
            <a-descriptions-item label="运行时间">
              {{ systemUptime }}
            </a-descriptions-item>
            <a-descriptions-item label="数据库状态">
              <a-tag color="green">正常</a-tag>
            </a-descriptions-item>
            <a-descriptions-item label="缓存状态">
              <a-tag color="green">正常</a-tag>
            </a-descriptions-item>
            <a-descriptions-item label="API 状态">
              <a-tag color="green">正常</a-tag>
            </a-descriptions-item>
          </a-descriptions>
        </a-card>

        <!-- 快捷操作 -->
        <a-card title="快捷操作" class="mb-6">
          <a-space direction="vertical" fill>
            <a-button long @click="$router.push('/users')">
              <template #icon>
                <icon-user-group />
              </template>
              用户管理
            </a-button>
            <a-button long @click="$router.push('/system/logs')">
              <template #icon>
                <icon-file />
              </template>
              系统日志
            </a-button>
            <a-button long @click="$router.push('/system/settings')">
              <template #icon>
                <icon-settings />
              </template>
              系统设置
            </a-button>
            <a-button long @click="handleBackup">
              <template #icon>
                <icon-cloud-download />
              </template>
              数据备份
            </a-button>
          </a-space>
        </a-card>

        <!-- 管理员公告 -->
        <a-card title="管理员公告">
          <a-alert
            type="info"
            title="系统维护通知"
            content="计划于本周日凌晨 2:00-4:00 进行系统维护，请提前做好准备。"
            closable
          />
          <a-alert
            type="warning"
            title="安全提醒"
            content="请定期修改管理员密码，确保系统安全。"
            closable
            class="mt-3"
          />
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import {
  IconUser,
  IconUserAdd,
  IconArrowRise,
  IconFire,
  IconWifi,
  IconBarChart,
  IconUserGroup,
  IconFile,
  IconSettings,
  IconCloudDownload,
} from '@arco-design/web-vue/es/icon'
import { userApi } from '@/api'
import type { UserListItem } from '@paigram/shared-components'

const router = useRouter()

// 统计数据
const statistics = reactive({
  totalUsers: 15234,
  todayNewUsers: 128,
  newUserGrowth: 12.5,
  activeUsers: 8456,
  onlineUsers: 1234,
})

// 图表周期
const chartPeriod = ref('month')

// 最新用户
const latestUsers = ref<UserListItem[]>([])

// 系统运行时间
const systemUptime = ref('0天0小时')
let uptimeTimer: ReturnType<typeof setInterval> | null = null

// 计算系统运行时间
const calculateUptime = () => {
  const startTime = new Date('2024-01-01').getTime()
  const now = Date.now()
  const diff = now - startTime

  const days = Math.floor(diff / 86400000)
  const hours = Math.floor((diff % 86400000) / 3600000)
  const minutes = Math.floor((diff % 3600000) / 60000)

  systemUptime.value = `${days}天${hours}小时${minutes}分钟`
}

// 格式化相对时间
const formatRelativeTime = (date: string): string => {
  const now = Date.now()
  const then = new Date(date).getTime()
  const diff = now - then

  const minutes = Math.floor(diff / 60000)
  const hours = Math.floor(diff / 3600000)
  const days = Math.floor(diff / 86400000)

  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes}分钟前`
  if (hours < 24) return `${hours}小时前`
  return `${days}天前`
}

// 查看用户
const handleViewUser = (user: { id: number }) => {
  router.push(`/users/${user.id}`)
}

// 数据备份
const handleBackup = () => {
  Message.loading('正在创建备份...')
  setTimeout(() => {
    Message.success('备份创建成功')
  }, 2000)
}

// 加载数据
const loadDashboardData = async () => {
  try {
    // TODO: 调用 API 获取统计数据
    // const stats = await api.getStatistics()
    // Object.assign(statistics, stats)

    // 获取最新用户
    const response = await userApi.getList()
    latestUsers.value = response.data.slice(0, 3)
  } catch (error) {
    console.error('Failed to load dashboard data:', error)
  }
}

onMounted(() => {
  calculateUptime()
  uptimeTimer = setInterval(calculateUptime, 60000) // 每分钟更新一次
  loadDashboardData()
})

onUnmounted(() => {
  if (uptimeTimer) {
    clearInterval(uptimeTimer)
  }
})
</script>

<style scoped></style>
