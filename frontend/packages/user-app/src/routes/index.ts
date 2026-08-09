import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

// 公共路由
export const constantRoutes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'Home',
    component: () => import('@/pages/home/index.vue'),
    meta: { locale: 'common.home', requiresAuth: false },
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/pages/login/index.vue'),
    meta: { locale: 'common.login', requiresAuth: false, hideInMenu: true },
  },
  {
    path: '/register',
    name: 'Register',
    component: () => import('@/pages/register/index.vue'),
    meta: { locale: 'common.register', requiresAuth: false, hideInMenu: true },
  },
  {
    path: '/forgot-password',
    name: 'ForgotPassword',
    component: () => import('@/pages/auth/forgot-password.vue'),
    meta: { locale: 'common.forgotPassword', requiresAuth: false, hideInMenu: true },
  },
  {
    path: '/verify-email',
    name: 'VerifyEmail',
    component: () => import('@/pages/auth/verify-email.vue'),
    meta: { locale: 'common.verifyEmail', requiresAuth: false, hideInMenu: true },
  },
  {
    path: '/auth/callback/:provider',
    name: 'OAuthCallback',
    component: () => import('@/pages/auth/oauth-callback.vue'),
    meta: { requiresAuth: false, hideInMenu: true },
  },
  {
    path: '/404',
    name: 'NotFound',
    component: () => import('@/pages/error/404.vue'),
    meta: { locale: 'error.404', requiresAuth: false, hideInMenu: true },
  },
  {
    path: '/403',
    name: 'NoPermission',
    component: () => import('@/pages/error/403.vue'),
    meta: { locale: 'error.403', requiresAuth: false, hideInMenu: true },
  },
]

// 需要根据权限动态加载的路由
export const asyncRoutes: RouteRecordRaw[] = [
  {
    path: '/dashboard',
    name: 'DashboardLayout',
    component: () => import('@/layouts/index.vue'),
    redirect: { name: 'Dashboard' },
    children: [
      {
        path: '',
        name: 'Dashboard',
        component: () => import('@/pages/dashboard/index.vue'),
        meta: { locale: 'menu.dashboard', icon: 'icon-dashboard', requiresAuth: true },
      },
    ],
  },
  {
    path: '/profile',
    name: 'ProfileLayout',
    component: () => import('@/layouts/index.vue'),
    children: [
      {
        path: '',
        name: 'Profile',
        component: () => import('@/pages/profile/index.vue'),
        meta: { locale: 'menu.profile', icon: 'icon-user', hideInMenu: true, requiresAuth: true },
      },
    ],
  },
  {
    path: '/account',
    name: 'Account',
    component: () => import('@/layouts/index.vue'),
    redirect: { name: 'AccountInfo' },
    meta: { locale: 'menu.account', icon: 'icon-idcard', requiresAuth: true },
    children: [
      {
        path: 'info',
        name: 'AccountInfo',
        component: () => import('@/pages/account/info.vue'),
        meta: { locale: 'menu.account.info', requiresAuth: true },
      },
      {
        path: 'security',
        name: 'AccountSecurity',
        component: () => import('@/pages/account/security.vue'),
        meta: { locale: 'menu.account.security', requiresAuth: true },
      },
      {
        path: 'binding',
        name: 'AccountBinding',
        component: () => import('@/pages/account/binding.vue'),
        meta: { locale: 'menu.account.binding', requiresAuth: true },
      },
      {
        path: 'logs',
        name: 'AccountLogs',
        component: () => import('@/pages/account/logs.vue'),
        meta: { locale: 'menu.account.logs', requiresAuth: true },
      },
    ],
  },
  {
    path: '/settings',
    name: 'Settings',
    component: () => import('@/layouts/index.vue'),
    redirect: { name: 'GeneralSettings' },
    meta: { locale: 'menu.settings', icon: 'icon-settings', requiresAuth: true },
    children: [
      {
        path: 'general',
        name: 'GeneralSettings',
        component: () => import('@/pages/settings/general.vue'),
        meta: { locale: 'menu.settings.general', requiresAuth: true },
      },
      {
        path: 'security',
        name: 'SecuritySettings',
        component: () => import('@/pages/settings/security.vue'),
        meta: { locale: 'menu.settings.security', requiresAuth: true },
      },
    ],
  },
  {
    path: '/apps',
    name: 'Apps',
    component: () => import('@/layouts/index.vue'),
    children: [
      {
        path: '',
        name: 'AppsList',
        component: () => import('@/pages/apps/index.vue'),
        meta: { locale: 'menu.apps', icon: 'icon-apps', requiresAuth: true },
      },
    ],
  },
]

// 捕获所有未匹配的路由
const catchAllRoute: RouteRecordRaw = {
  path: '/:pathMatch(.*)*',
  redirect: '/404',
}

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [...constantRoutes, ...asyncRoutes, catchAllRoute],
  scrollBehavior: () => ({ left: 0, top: 0 }),
})

// 设置路由守卫（已移至 main.ts，确保 pinia 先注册）

export default router
