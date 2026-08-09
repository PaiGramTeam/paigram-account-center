export interface RouteMeta {
  title?: string
  locale?: string
  icon?: string
  hidden?: boolean
  hideInMenu?: boolean
  hideChildrenInMenu?: boolean
  activeMenu?: string
  requiresAuth?: boolean
  permissions?: string[]
  roles?: string[]
  keepAlive?: boolean
  ignoreCache?: boolean
  noAffix?: boolean
  order?: number
  sort?: number
  disabled?: boolean
}

export interface MenuItem {
  path: string
  name: string
  meta: RouteMeta
  children?: MenuItem[]
}

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

export interface AppConfig {
  title: string
  logo?: string
  theme: 'light' | 'dark' | 'auto'
  primaryColor: string
  collapsed: boolean
  showFooter: boolean
  showBreadcrumb: boolean
}
