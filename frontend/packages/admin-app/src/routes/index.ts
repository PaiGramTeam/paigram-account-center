import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

export const constantRoutes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/pages/login/index.vue'),
    meta: { locale: 'common.login', requiresAuth: false, hideInMenu: true },
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

export const asyncRoutes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'Layout',
    component: () => import('@/layouts/AdminLayout.vue'),
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('@/pages/dashboard/index.vue'),
        meta: {
          locale: 'menu.dashboard',
          icon: 'icon-dashboard',
          requiresAuth: true,
        },
      },
      {
        path: 'users',
        name: 'Users',
        redirect: '/users/list',
        meta: {
          locale: 'menu.users',
          icon: 'icon-user-group',
          requiresAuth: true,
          permissions: ['user:read', 'role:read', 'permission:read'],
        },
        children: [
          {
            path: 'list',
            name: 'UserList',
            component: () => import('@/pages/users/index.vue'),
            meta: {
              locale: 'menu.users.list',
              permissions: ['user:read'],
              requiresAuth: true,
            },
          },
          {
            path: ':id/detail',
            name: 'UserDetail',
            component: () => import('@/pages/users/detail.vue'),
            meta: {
              locale: 'menu.users.detail',
              permissions: ['user:read'],
              requiresAuth: true,
              hideInMenu: true,
            },
          },
          {
            path: 'roles',
            name: 'UserRoles',
            component: () => import('@/pages/users/roles.vue'),
            meta: {
              locale: 'menu.users.roles',
              permissions: ['role:read'],
              requiresAuth: true,
            },
          },
          {
            path: 'permissions',
            name: 'UserPermissions',
            component: () => import('@/pages/users/permissions.vue'),
            meta: {
              locale: 'menu.users.permissions',
              permissions: ['permission:read'],
              requiresAuth: true,
            },
          },
        ],
      },
      {
        path: 'platform-accounts',
        name: 'PlatformAccounts',
        component: () => import('@/pages/platform-accounts/index.vue'),
        meta: {
          locale: 'menu.platformAccounts',
          icon: 'icon-cloud',
          requiresAuth: true,
          permissions: ['platform_account:list'],
        },
      },
      {
        path: 'services',
        name: 'Services',
        component: () => import('@/pages/services/index.vue'),
        meta: {
          locale: 'menu.services',
          icon: 'icon-apps',
          requiresAuth: true,
          permissions: ['platform:list', 'bot:list'],
        },
      },
      {
        path: 'system',
        name: 'System',
        redirect: '/system/logs',
        meta: {
          locale: 'menu.system',
          icon: 'icon-settings',
          requiresAuth: true,
          permissions: ['system:read', 'audit:list'],
        },
        children: [
          {
            path: 'settings',
            name: 'SystemSettings',
            component: () => import('@/pages/system/settings.vue'),
            meta: {
              locale: 'menu.system.settings',
              permissions: ['system:read'],
              requiresAuth: true,
            },
          },
          {
            path: 'logs',
            name: 'SystemLogs',
            component: () => import('@/pages/system/logs.vue'),
            meta: {
              locale: 'menu.system.logs',
              permissions: ['audit:list'],
              requiresAuth: true,
            },
          },
        ],
      },
      {
        path: 'profile',
        name: 'Profile',
        component: () => import('@/pages/profile/index.vue'),
        meta: {
          locale: 'menu.profile',
          icon: 'icon-user',
          requiresAuth: true,
          hideInMenu: true,
        },
      },
    ],
  },
]

const catchAllRoute: RouteRecordRaw = {
  path: '/:pathMatch(.*)*',
  redirect: '/404',
}

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [...constantRoutes, ...asyncRoutes, catchAllRoute],
  scrollBehavior: () => ({ left: 0, top: 0 }),
})

export default router
