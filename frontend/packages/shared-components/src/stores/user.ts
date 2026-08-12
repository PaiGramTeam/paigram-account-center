import { defineStore } from 'pinia'
import type { UserInfo } from '../api/types'

type RemoteLogout = () => Promise<void>

let remoteLogout: RemoteLogout | undefined

export function configureUserLogout(handler: RemoteLogout): void {
  remoteLogout = handler
}

export interface UserState {
  userInfo: UserInfo | null
  token: string
  sessionEpoch: number
}

export const useUserStore = defineStore('user', {
  state: (): UserState => ({
    userInfo: null,
    token: '',
    sessionEpoch: 0,
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

    setAuthData(data: { accessToken: string }) {
      this.token = data.accessToken
    },

    setToken(token: string) {
      this.token = token
    },

    async logout() {
      try {
        if (remoteLogout) {
          await remoteLogout()
        }
      } finally {
        this.reset()
      }
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
      if (this.permissions.includes('*')) {
        return true
      }

      if (this.permissions.includes(permission)) {
        return true
      }

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
      this.sessionEpoch += 1
    },
  },
})
