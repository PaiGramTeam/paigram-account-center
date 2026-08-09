// 路由元信息类型
export interface RouteMeta {
  title?: string
  locale?: string // 菜单名称的语言包键名
  icon?: string
  hidden?: boolean
  hideInMenu?: boolean // 是否在左侧菜单中隐藏该项
  hideChildrenInMenu?: boolean // 强制在左侧菜单中显示单项
  activeMenu?: string // 高亮设置的菜单项
  requiresAuth?: boolean
  permissions?: string[]
  roles?: string[]
  keepAlive?: boolean
  ignoreCache?: boolean // 如果设置为true页面将不会被缓存
  noAffix?: boolean // 如果设置为true，标签将不会添加到tab-bar中
  order?: number // 排序路由菜单项。如果设置该值，值越高，越靠前
  sort?: number
  disabled?: boolean // 是否禁用菜单项
}

// 菜单项类型
export interface MenuItem {
  path: string
  name: string
  meta: RouteMeta
  children?: MenuItem[]
}

// 用户信息类型
export interface UserInfo {
  id: number | string
  username: string
  nickname: string
  email?: string
  avatar?: string
  roles: string[]
  permissions: string[]
  status?: string
  created_at?: string
  updated_at?: string
}

// 应用配置类型
export interface AppConfig {
  title: string
  logo?: string
  theme: 'light' | 'dark' | 'auto'
  primaryColor: string
  collapsed: boolean
  showFooter: boolean
  showBreadcrumb: boolean
}
