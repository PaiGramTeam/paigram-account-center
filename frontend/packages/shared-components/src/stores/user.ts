import { defineStore } from 'pinia'
import type { UserInfo } from '../api/types'

export interface UserState {
  userInfo: UserInfo | null
  token: string
  refreshToken: string
}

export const useUserStore = defineStore('user', {
  state: (): UserState => ({
    userInfo: null,
    token: '',
    refreshToken: '',
  }),

  getters: {
    isLogin: (state) => !!state.token,
    userId: (state) => state.userInfo?.id || '',
    displayName: (state) => state.userInfo?.display_name || '',
    username: (state) => state.userInfo?.display_name || '', // For compatibility
    nickname: (state) => state.userInfo?.display_name || '', // For compatibility
    email: (state) => state.userInfo?.primary_email || '',
    avatar: (state) => state.userInfo?.avatar_url || '',
    roles: (state) => state.userInfo?.roles || [],
    permissions: (state) => state.userInfo?.permissions || [],
  },

  actions: {
    setUserInfo(userInfo: UserInfo) {
      this.userInfo = userInfo
    },

    setAuthData(data: { accessToken: string; refreshToken: string }) {
      this.token = data.accessToken
      this.refreshToken = data.refreshToken
    },

    setToken(token: string, refreshToken?: string) {
      this.token = token
      if (refreshToken) {
        this.refreshToken = refreshToken
      }
    },

    async login(loginForm: { email: string; password: string }) {
      // 实际的登录 API 调用需要在应用中实现
      // 这里只是模拟
      this.setToken('mock-token', 'mock-refresh-token')
      this.setUserInfo({
        id: 1,
        display_name: '用户昵称',
        primary_email: loginForm.email,
        avatar_url: '',
        status: 'active',
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        roles: ['user'],
        permissions: ['read'],
      })
    },

    async logout() {
      // TODO: 调用登出 API
      this.reset()
    },

    async fetchUserInfo() {
      // fetchUserInfo is now handled by individual apps (authStore.fetchUserProfile)
      // This method is kept for compatibility with router guard interface
      // If userInfo is already set, do nothing
      if (this.userInfo) {
        return
      }
      // Otherwise, this should be called from app-specific auth store
      throw new Error('User info must be fetched before calling fetchUserInfo')
    },

    hasPermission(permission: string): boolean {
      // 如果有 * 通配符，拥有所有权限
      if (this.permissions.includes('*')) {
        return true
      }

      // 精确匹配权限
      if (this.permissions.includes(permission)) {
        return true
      }

      // 检查通配符模式 (例如 user:* 匹配 user:read)
      return this.permissions.some((p) => {
        if (p.endsWith(':*')) {
          const prefix = p.slice(0, -2)
          return permission.startsWith(prefix + ':')
        }
        return false
      })
    },

    hasRole(role: string): boolean {
      return this.roles.includes(role) || this.roles.includes('admin')
    },

    reset() {
      this.userInfo = null
      this.token = ''
      this.refreshToken = ''
    },
  },

  persist: {
    key: 'paigram-user-store',
    pick: ['token', 'refreshToken', 'userInfo'],
  },
})
