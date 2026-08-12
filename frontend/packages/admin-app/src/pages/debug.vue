<template>
  <div class="space-y-6 p-6">
    <a-card title="🔍 前端调试面板">
      <template #extra>
        <a-button type="primary" @click="refreshData">刷新数据</a-button>
      </template>

      <div class="mb-6">
        <h3 class="mb-4 text-lg font-semibold">👤 用户信息</h3>
        <a-descriptions :column="2" bordered>
          <a-descriptions-item label="是否登录">
            <a-tag :color="userStore.isLogin ? 'green' : 'red'">
              {{ userStore.isLogin ? '已登录' : '未登录' }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="用户ID">
            {{ userStore.userInfo?.id || '无' }}
          </a-descriptions-item>
          <a-descriptions-item label="用户名">
            {{ userStore.userInfo?.display_name || '无' }}
          </a-descriptions-item>
          <a-descriptions-item label="邮箱">
            {{ userStore.userInfo?.primary_email || '无' }}
          </a-descriptions-item>
          <a-descriptions-item label="Token">
            <a-typography-text copyable>
              {{ userStore.token ? userStore.token.substring(0, 30) + '...' : '无' }}
            </a-typography-text>
          </a-descriptions-item>
        </a-descriptions>
      </div>

      <div class="mb-6">
        <h3 class="mb-4 text-lg font-semibold">🔐 权限信息</h3>
        <a-alert v-if="!userStore.userInfo?.permissions?.length" type="warning" class="mb-4">
          <template #title>警告：用户没有任何权限数据</template>
          这可能导致菜单显示异常。请检查后端是否返回了权限字段。
        </a-alert>

        <div class="mb-4">
          <strong>角色列表 ({{ userStore.userInfo?.roles?.length || 0 }}):</strong>
          <div class="mt-2">
            <a-tag v-for="role in userStore.userInfo?.roles" :key="role" color="blue" class="mb-2">
              {{ role }}
            </a-tag>
            <a-tag v-if="!userStore.userInfo?.roles?.length" color="gray">无角色</a-tag>
          </div>
        </div>

        <div>
          <strong>权限列表 ({{ userStore.userInfo?.permissions?.length || 0 }}):</strong>
          <div class="mt-2 max-h-60 overflow-y-auto">
            <a-tag v-for="perm in userStore.userInfo?.permissions" :key="perm" color="green" class="mb-2">
              {{ perm }}
            </a-tag>
            <a-tag v-if="!userStore.userInfo?.permissions?.length" color="gray">无权限</a-tag>
          </div>
        </div>
      </div>

      <div class="mb-6">
        <h3 class="mb-4 text-lg font-semibold">🗺️ 路由信息</h3>
        <a-descriptions :column="1" bordered>
          <a-descriptions-item label="总路由数">
            {{ router.options.routes.length }}
          </a-descriptions-item>
          <a-descriptions-item label="当前路径">
            {{ route.path }}
          </a-descriptions-item>
          <a-descriptions-item label="当前路由名称">
            {{ route.name }}
          </a-descriptions-item>
        </a-descriptions>
      </div>

      <div class="mb-6">
        <h3 class="mb-4 text-lg font-semibold">📋 菜单路由权限要求</h3>
        <a-table :data="routePermissions" :pagination="false" bordered>
          <template #columns>
            <a-table-column title="路由名称" data-index="name" />
            <a-table-column title="路径" data-index="path" />
            <a-table-column title="需要的权限" data-index="permissions">
              <template #cell="{ record }">
                <a-tag v-for="perm in record.permissions" :key="perm" color="purple" class="mb-1">
                  {{ perm }}
                </a-tag>
                <a-tag v-if="!record.permissions?.length" color="gray">无权限要求</a-tag>
              </template>
            </a-table-column>
            <a-table-column title="用户是否有权限" data-index="hasAccess">
              <template #cell="{ record }">
                <a-tag :color="record.hasAccess ? 'green' : 'red'">
                  {{ record.hasAccess ? '✅ 有权限' : '❌ 无权限' }}
                </a-tag>
              </template>
            </a-table-column>
          </template>
        </a-table>
      </div>

      <div>
        <h3 class="mb-4 text-lg font-semibold">📦 完整的 UserInfo 对象</h3>
        <a-textarea
          :model-value="JSON.stringify(userStore.userInfo, null, 2)"
          :auto-size="{ minRows: 10, maxRows: 20 }"
          readonly
        />
      </div>

      <div class="mt-6 flex gap-4">
        <a-button type="primary" @click="mockAdminPermissions">模拟管理员权限</a-button>
        <a-button @click="mockUserPermissions">模拟普通用户权限</a-button>
        <a-button status="danger" @click="clearPermissions">清空权限</a-button>
        <a-button @click="copyToClipboard">复制调试信息</a-button>
      </div>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { useUserStore } from '@paigram/shared-components'
import { Message } from '@arco-design/web-vue'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const routePermissions = computed(() => {
  const results: Array<{
    name: string
    path: string
    permissions: string[]
    hasAccess: boolean
  }> = []

  const userPermissions = userStore.userInfo?.permissions || []

  const processRoute = (route: RouteRecordRaw, parentPath = '') => {
    if (route.meta?.hideInMenu) return

    const fullPath = parentPath + route.path
    const permissions = (route.meta?.permissions as string[]) || []

    const hasAccess =
      permissions.length === 0 ||
      userPermissions.length === 0 ||
      permissions.some((perm) => userPermissions.includes(perm))

    results.push({
      name: route.name as string,
      path: fullPath,
      permissions,
      hasAccess,
    })

    if (route.children) {
      route.children.forEach((child) => processRoute(child, fullPath + '/'))
    }
  }

  router.options.routes.forEach((route) => processRoute(route))
  return results.filter((r) => r.name)
})

const refreshData = () => {
  Message.info('数据已刷新')
  console.log('UserStore:', userStore.$state)
}

const mockAdminPermissions = () => {
  if (userStore.userInfo) {
    userStore.userInfo.permissions = [
      'user:create',
      'user:read',
      'user:update',
      'user:delete',
      'user:list',
      'role:create',
      'role:read',
      'role:update',
      'role:delete',
      'role:list',
      'role:manage',
      'permission:read',
      'permission:list',
      'audit:read',
      'audit:list',
    ]
    userStore.userInfo.roles = ['admin']
    Message.success('已设置管理员权限，菜单应该全部显示')
  } else {
    Message.error('请先登录')
  }
}

const mockUserPermissions = () => {
  if (userStore.userInfo) {
    userStore.userInfo.permissions = ['user:read', 'audit:read']
    userStore.userInfo.roles = ['user']
    Message.success('已设置普通用户权限，只显示部分菜单')
  } else {
    Message.error('请先登录')
  }
}

const clearPermissions = () => {
  if (userStore.userInfo) {
    userStore.userInfo.permissions = []
    userStore.userInfo.roles = []
    Message.warning('已清空权限，应该显示所有菜单（开发模式）')
  } else {
    Message.error('请先登录')
  }
}

const copyToClipboard = async () => {
  const debugInfo = {
    userInfo: userStore.userInfo,
    token: userStore.token ? userStore.token.substring(0, 30) + '...' : null,
    routes: router.options.routes.length,
    currentPath: route.path,
    routePermissions: routePermissions.value,
  }

  try {
    await navigator.clipboard.writeText(JSON.stringify(debugInfo, null, 2))
    Message.success('调试信息已复制到剪贴板')
  } catch (_err) {
    Message.error('复制失败')
  }
}
</script>
