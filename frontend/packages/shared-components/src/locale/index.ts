import { createI18n } from 'vue-i18n'
import type { App } from 'vue'
import zhCN from './zh-CN'
import enUS from './en-US'

export type LocaleType = 'zh-CN' | 'en-US'

const defaultLocale: LocaleType = 'zh-CN'

const i18n = createI18n({
  locale: defaultLocale,
  fallbackLocale: 'en-US',
  legacy: false,
  allowComposition: true,
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
})

export const setupI18n = (app: App) => {
  app.use(i18n)
}

export const t: typeof i18n.global.t = i18n.global.t
export const locale: typeof i18n.global.locale = i18n.global.locale

export const changeLocale = (locale: LocaleType) => {
  i18n.global.locale.value = locale
  localStorage.setItem('locale', locale)
  // Update Arco Design locale
  if (locale === 'zh-CN') {
    import('@arco-design/web-vue/es/locale/lang/zh-cn').then((module) => {
      ;(window as Window & { $arcoLang?: unknown }).$arcoLang = module.default
    })
  } else {
    import('@arco-design/web-vue/es/locale/lang/en-us').then((module) => {
      ;(window as Window & { $arcoLang?: unknown }).$arcoLang = module.default
    })
  }
}

// Initialize locale from localStorage
const savedLocale = localStorage.getItem('locale') as LocaleType
if (savedLocale && ['zh-CN', 'en-US'].includes(savedLocale)) {
  changeLocale(savedLocale)
}

export default i18n
