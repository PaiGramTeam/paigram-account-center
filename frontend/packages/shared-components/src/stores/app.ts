import { defineStore } from 'pinia'
import type { AppConfig } from '../types'

const THEME_STORAGE_KEY = 'paigram-theme-preference'

// 获取系统主题偏好
function getSystemTheme(): 'light' | 'dark' {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

// 应用主题到 DOM
function applyTheme(theme: 'light' | 'dark') {
  if (theme === 'dark') {
    document.body.setAttribute('arco-theme', 'dark')
  } else {
    document.body.removeAttribute('arco-theme')
  }
}

export const useAppStore = defineStore('app', {
  state: (): AppConfig & { systemThemeListener?: () => void } => ({
    title: 'Paigram Account Center',
    logo: '',
    theme: 'auto', // 默认跟随系统
    primaryColor: '#165DFF',
    collapsed: false,
    showFooter: true,
    showBreadcrumb: true,
    systemThemeListener: undefined,
  }),

  getters: {
    // 获取实际应用的主题（解析 auto）
    effectiveTheme(): 'light' | 'dark' {
      if (this.theme === 'auto') {
        return getSystemTheme()
      }
      return this.theme
    },
  },

  actions: {
    toggleCollapsed() {
      this.collapsed = !this.collapsed
    },

    setCollapsed(collapsed: boolean) {
      this.collapsed = collapsed
    },

    setTheme(theme: 'light' | 'dark' | 'auto') {
      this.theme = theme

      // 保存到 localStorage
      localStorage.setItem(THEME_STORAGE_KEY, theme)

      // 应用实际主题
      const effectiveTheme = theme === 'auto' ? getSystemTheme() : theme
      applyTheme(effectiveTheme)

      // 如果是 auto 模式，设置监听器
      if (theme === 'auto') {
        this.setupSystemThemeListener()
      } else {
        this.removeSystemThemeListener()
      }
    },

    // 设置系统主题监听器
    setupSystemThemeListener() {
      if (typeof window === 'undefined') return

      // 移除旧的监听器
      this.removeSystemThemeListener()

      const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
      const listener = (e: MediaQueryListEvent) => {
        if (this.theme === 'auto') {
          applyTheme(e.matches ? 'dark' : 'light')
        }
      }

      mediaQuery.addEventListener('change', listener)
      this.systemThemeListener = () => {
        mediaQuery.removeEventListener('change', listener)
      }
    },

    // 移除系统主题监听器
    removeSystemThemeListener() {
      if (this.systemThemeListener) {
        this.systemThemeListener()
        this.systemThemeListener = undefined
      }
    },

    // 初始化主题（从 localStorage 读取）
    initTheme() {
      const savedTheme = localStorage.getItem(THEME_STORAGE_KEY) as 'light' | 'dark' | 'auto' | null

      if (savedTheme && ['light', 'dark', 'auto'].includes(savedTheme)) {
        this.setTheme(savedTheme)
      } else {
        // 默认使用 auto 模式
        this.setTheme('auto')
      }
    },

    setPrimaryColor(color: string) {
      this.primaryColor = color
      document.body.style.setProperty('--primary-6', color)
    },

    updateSettings(settings: Partial<AppConfig>) {
      Object.assign(this, settings)
    },

    toggleBinaryTheme() {
      this.setTheme(this.effectiveTheme === 'dark' ? 'light' : 'dark')
    },

    cycleThemeMode() {
      // 循环切换：light -> dark -> auto -> light
      if (this.theme === 'light') {
        this.setTheme('dark')
      } else if (this.theme === 'dark') {
        this.setTheme('auto')
      } else {
        this.setTheme('light')
      }
    },
  },
})
