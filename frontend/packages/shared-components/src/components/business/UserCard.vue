<template>
  <a-card :hoverable="hoverable" :class="cardClass">
    <div class="flex items-start space-x-4">
      <a-avatar :size="avatarSize" class="shrink-0">
        <img v-if="user.avatar_url" :src="user.avatar_url" :alt="user.display_name" />
        <icon-user v-else />
      </a-avatar>

      <div class="min-w-0 flex-1">
        <div class="flex items-start justify-between">
          <div class="flex-1">
            <h3 class="truncate text-lg font-semibold text-gray-900 dark:text-gray-100">
              {{ user.display_name }}
            </h3>
            <p class="truncate text-sm text-gray-500 dark:text-gray-400">
              {{ user.primary_email }}
            </p>
          </div>

          <a-tag :color="getStatusColor(user.status)" size="small">
            {{ getStatusText(user.status) }}
          </a-tag>
        </div>

        <div v-if="showDetails" class="mt-3 space-y-2">
          <div class="flex items-center text-sm text-gray-600 dark:text-gray-400">
            <icon-idcard class="mr-2" />
            <span>ID: {{ user.id }}</span>
          </div>

          <div v-if="user.bio" class="flex items-start text-sm text-gray-600 dark:text-gray-400">
            <icon-info-circle class="mt-0.5 mr-2" />
            <span class="line-clamp-2">{{ user.bio }}</span>
          </div>

          <div class="flex items-center text-sm text-gray-600 dark:text-gray-400">
            <icon-calendar class="mr-2" />
            <span>注册于 {{ formatDate(user.created_at) }}</span>
          </div>

          <div v-if="user.last_login_at" class="flex items-center text-sm text-gray-600 dark:text-gray-400">
            <icon-clock-circle class="mr-2" />
            <span>最后登录 {{ formatRelativeTime(user.last_login_at) }}</span>
          </div>
        </div>

        <div v-if="showRoles && user.roles?.length" class="mt-3">
          <a-space wrap :size="8">
            <a-tag v-for="role in user.roles" :key="role" size="small">
              {{ role }}
            </a-tag>
          </a-space>
        </div>

        <div v-if="showActions" class="mt-4">
          <a-space>
            <a-button v-if="canView" size="small" @click="handleView"> 查看详情 </a-button>
            <a-button v-if="canEdit" size="small" @click="handleEdit"> 编辑 </a-button>
            <a-dropdown v-if="canMessage || canBlock">
              <a-button size="small">
                更多
                <icon-down />
              </a-button>
              <template #content>
                <a-doption v-if="canMessage" @click="handleMessage">
                  <template #icon>
                    <icon-message />
                  </template>
                  发送消息
                </a-doption>
                <a-doption
                  v-if="canBlock"
                  @click="handleBlock"
                  :class="{ 'text-red-500': user.status !== 'suspended' }"
                >
                  <template #icon>
                    <icon-stop v-if="user.status !== 'suspended'" />
                    <icon-check v-else />
                  </template>
                  {{ user.status === 'suspended' ? '解除封禁' : '封禁用户' }}
                </a-doption>
              </template>
            </a-dropdown>
          </a-space>
        </div>
      </div>
    </div>
  </a-card>
</template>

<script setup lang="ts">
import {
  IconUser,
  IconIdcard,
  IconInfoCircle,
  IconCalendar,
  IconClockCircle,
  IconDown,
  IconMessage,
  IconStop,
  IconCheck,
} from '@arco-design/web-vue/es/icon'
import type { UserInfo } from '../../api/types'

interface Props {
  user: UserInfo

  showDetails?: boolean
  showRoles?: boolean
  showActions?: boolean
  hoverable?: boolean

  canView?: boolean
  canEdit?: boolean
  canMessage?: boolean
  canBlock?: boolean

  avatarSize?: number
  cardClass?: string
}

const props = withDefaults(defineProps<Props>(), {
  showDetails: true,
  showRoles: true,
  showActions: true,
  hoverable: true,
  canView: true,
  canEdit: true,
  canMessage: false,
  canBlock: true,
  avatarSize: 64,
  cardClass: '',
})

const emit = defineEmits<{
  view: [user: UserInfo]
  edit: [user: UserInfo]
  message: [user: UserInfo]
  block: [user: UserInfo]
}>()

const getStatusColor = (status: string): string => {
  const colorMap: Record<string, string> = {
    active: 'green',
    pending: 'blue',
    suspended: 'red',
    deleted: 'gray',
  }
  return colorMap[status] || 'gray'
}

const getStatusText = (status: string): string => {
  const textMap: Record<string, string> = {
    active: '正常',
    pending: '待处理',
    suspended: '已停用',
    deleted: '已删除',
  }
  return textMap[status] || '未知'
}

const formatDate = (date?: string): string => {
  if (!date) return '-'
  return new Date(date).toLocaleDateString('zh-CN')
}

const formatRelativeTime = (date?: string): string => {
  if (!date) return '-'

  const now = Date.now()
  const then = new Date(date).getTime()
  const diff = now - then

  const minutes = Math.floor(diff / 60000)
  const hours = Math.floor(diff / 3600000)
  const days = Math.floor(diff / 86400000)

  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes}分钟前`
  if (hours < 24) return `${hours}小时前`
  if (days < 30) return `${days}天前`
  return formatDate(date)
}

const handleView = () => {
  emit('view', props.user)
}

const handleEdit = () => {
  emit('edit', props.user)
}

const handleMessage = () => {
  emit('message', props.user)
}

const handleBlock = () => {
  emit('block', props.user)
}
</script>

<style scoped></style>
