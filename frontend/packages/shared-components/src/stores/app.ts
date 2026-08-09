import { defineStore } from 'pinia'
import type { AppConfig } from '../types'

const THEME_STORAGE_KEY = 'paigram-theme-preference'

function getSystemTheme(): 'light' | 'dark' {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

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
    theme: 'auto',
    primaryColor: '#165DFF',
    collapsed: false,
    showFooter: true,
    showBreadcrumb: true,
    systemThemeListener: undefined,
  }),

  getters: {
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

      localStorage.setItem(THEME_STORAGE_KEY, theme)

      const effectiveTheme = theme === 'auto' ? getSystemTheme() : theme
      applyTheme(effectiveTheme)

      if (theme === 'auto') {
        this.setupSystemThemeListener()
      } else {
        this.removeSystemThemeListener()
      }
    },

    setupSystemThemeListener() {
      if (typeof window === 'undefined') return

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

    removeSystemThemeListener() {
      if (this.systemThemeListener) {
        this.systemThemeListener()
        this.systemThemeListener = undefined
      }
    },

    initTheme() {
      const savedTheme = localStorage.getItem(THEME_STORAGE_KEY) as 'light' | 'dark' | 'auto' | null

      if (savedTheme && ['light', 'dark', 'auto'].includes(savedTheme)) {
        this.setTheme(savedTheme)
      } else {
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
