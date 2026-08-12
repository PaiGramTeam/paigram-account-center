import { delay, http, HttpResponse } from 'msw'
import type {
  OpenApiAdminUserListResponse,
  OpenApiCurrentUser,
  OpenApiLoginMethod,
  OpenApiLoginResponse,
  OpenApiMeResponse,
  OpenApiRole,
  OpenApiRoleListResponse,
  OpenApiUserDetail,
} from '../api/openapi'
import type { MockOptions } from './config'

const now = '2026-08-09T00:00:00Z'

const currentUser = {
  id: 1,
  display_name: 'Mock User',
  primary_email: 'mock@example.test',
  avatar_url: '',
  bio: 'Local mock account',
  locale: 'zh-CN',
  status: 'active',
  created_at: now,
  updated_at: now,
  last_login_at: now,
  emails: [{ id: 1, email: 'mock@example.test', is_primary: true, verified_at: now, created_at: now, updated_at: now }],
  login_methods: [
    {
      provider: 'email',
      provider_account_id: 'mock@example.test',
      display_name: 'Mock User',
      can_unbind: false,
      is_primary: true,
      created_at: now,
      updated_at: now,
    },
  ],
  roles: ['admin'],
  permissions: [
    'user:create',
    'user:read',
    'user:update',
    'user:delete',
    'user:list',
    'role:read',
    'permission:read',
    'audit:read',
  ],
} satisfies OpenApiCurrentUser

const adminUser = {
  id: currentUser.id,
  display_name: currentUser.display_name,
  primary_email: currentUser.primary_email,
  avatar_url: currentUser.avatar_url,
  bio: currentUser.bio,
  locale: currentUser.locale,
  status: currentUser.status,
  primary_login_type: 'email',
  created_at: now,
  updated_at: now,
  last_login_at: now,
  emails: currentUser.emails,
  roles: currentUser.roles,
  permissions: currentUser.permissions,
  two_factor_enabled: false,
  active_session_count: 1,
} satisfies OpenApiUserDetail

const role = {
  id: 1,
  name: 'admin',
  display_name: 'Mock Administrator',
  description: 'Administrator role used by browser mocks.',
  is_system: true,
  created_at: now,
  updated_at: now,
  permissions: [
    { id: 1, name: 'user:read', resource: 'user', action: 'read', description: 'Read users.' },
    { id: 2, name: 'role:read', resource: 'role', action: 'read', description: 'Read roles.' },
  ],
} satisfies OpenApiRole

const loginMethods = currentUser.login_methods satisfies OpenApiLoginMethod[]
const pagination = { page: 1, page_size: 20, total: 1, total_pages: 1 }

function ok<T>(data: T) {
  return { code: 0, message: 'OK', data }
}

async function applyScenario(options: MockOptions): Promise<Response | undefined> {
  if (options.scenario === 'slow') await delay(500)
  if (options.scenario === 'error') {
    return HttpResponse.json(
      { code: 'MOCK_SCENARIO_ERROR', message: 'The mock error scenario is active.' },
      { status: 503 }
    )
  }
  return undefined
}

export function createMockHandlers(options: MockOptions) {
  return [
    http.post('*/api/v1/auth/login', async () => {
      const scenarioResponse = await applyScenario(options)
      if (scenarioResponse) return scenarioResponse

      const response = ok({
        access_token: 'mock-access-token',
        refresh_token: 'mock-refresh-token',
        access_expiry: '2026-08-09T01:00:00Z',
        refresh_expiry: '2026-09-08T00:00:00Z',
        user_id: currentUser.id,
      }) satisfies OpenApiLoginResponse
      return HttpResponse.json(response)
    }),
    http.post('*/api/v1/auth/refresh', () =>
      HttpResponse.json(
        ok({
          access_token: 'mock-refreshed-access-token',
          refresh_token: 'mock-refreshed-refresh-token',
          access_expiry: '2026-08-09T01:00:00Z',
          refresh_expiry: '2026-09-08T00:00:00Z',
          user_id: currentUser.id,
        })
      )
    ),
    http.post('*/api/v1/auth/logout', () => HttpResponse.json(ok({ message: 'Logged out' }))),
    http.post('*/api/v1/auth/register', async ({ request }) => {
      const body = (await request.json()) as { email: string }
      return HttpResponse.json(ok({ user_id: currentUser.id, email: body.email, requires_email_verification: true }))
    }),
    http.post('*/api/v1/auth/verify-email', () => HttpResponse.json(ok({ message: 'Email verified' }))),
    http.post('*/api/v1/auth/oauth/:provider/init', ({ params }) =>
      HttpResponse.json(
        ok({
          auth_url: `https://example.test/oauth/${params.provider}`,
          state: 'mock-oauth-state',
          purpose: 'login',
          expires_at: now,
        })
      )
    ),
    http.post('*/api/v1/auth/oauth/:provider/callback', () =>
      HttpResponse.json(
        ok({
          access_token: 'mock-access-token',
          refresh_token: 'mock-refresh-token',
          access_expiry: '2026-08-09T01:00:00Z',
          refresh_expiry: '2026-09-08T00:00:00Z',
          user_id: currentUser.id,
        })
      )
    ),
    http.get('*/api/v1/me', async () => {
      const scenarioResponse = await applyScenario(options)
      if (scenarioResponse) return scenarioResponse
      const response = ok(currentUser) satisfies OpenApiMeResponse
      return HttpResponse.json(response)
    }),
    http.patch('*/api/v1/me', async ({ request }) => {
      const changes = (await request.json()) as Partial<OpenApiCurrentUser>
      Object.assign(currentUser, changes)
      return HttpResponse.json(ok(currentUser) satisfies OpenApiMeResponse)
    }),
    http.get('*/api/v1/me/login-methods', () => HttpResponse.json(ok(loginMethods))),
    http.put('*/api/v1/me/login-methods/:provider', ({ params }) =>
      HttpResponse.json(
        ok({
          auth_url: `https://example.test/oauth/${params.provider}`,
          state: 'mock-binding-state',
          purpose: 'bind',
          expires_at: now,
        })
      )
    ),
    http.delete('*/api/v1/me/login-methods/:provider', () =>
      HttpResponse.json(ok({ message: 'Login method removed' }))
    ),
    http.get('*/api/v1/me/activity-logs', () => HttpResponse.json(ok({ items: [], pagination }))),
    http.put('*/api/v1/me/security/password', () => HttpResponse.json(ok({ message: 'Password changed' }))),
    http.post('*/api/v1/me/security/2fa/setup', () =>
      HttpResponse.json(
        ok({
          qr_code: 'data:image/png;base64,',
          secret: 'MOCKTOTPSECRET',
          backup_codes: ['mock-backup-code'],
          expires_at: now,
        })
      )
    ),
    http.post('*/api/v1/me/security/2fa/confirm', () =>
      HttpResponse.json(ok({ message: 'Two-factor authentication enabled' }))
    ),
    http.delete('*/api/v1/me/security/2fa', () =>
      HttpResponse.json(ok({ message: 'Two-factor authentication disabled' }))
    ),
    http.get('*/api/v1/me/security/overview', () =>
      HttpResponse.json(
        ok({
          user_id: currentUser.id,
          two_factor_enabled: false,
          active_session_count: 1,
          device_count: 1,
          failed_logins_last_30_days: 0,
          last_login_at: now,
          last_login_ip: '127.0.0.1',
          last_login_device: 'Mock Browser',
        })
      )
    ),
    http.get('*/api/v1/me/sessions', () =>
      HttpResponse.json(
        ok({
          items: [
            {
              id: 1,
              device_id: 'mock-device',
              device_name: 'Mock Browser',
              device_type: 'desktop',
              ip: '127.0.0.1',
              created_at: now,
              last_active_at: now,
              access_expiry: now,
              refresh_expiry: now,
              is_current: true,
            },
          ],
          pagination,
        })
      )
    ),
    http.delete('*/api/v1/me/sessions/:sessionId', () => new HttpResponse(null, { status: 204 })),
    http.get('*/api/v1/admin/users', async () => {
      const scenarioResponse = await applyScenario(options)
      if (scenarioResponse) return scenarioResponse
      const response = ok({
        items: [
          {
            id: adminUser.id,
            display_name: adminUser.display_name,
            status: adminUser.status,
            primary_login_type: adminUser.primary_login_type,
            roles: adminUser.roles,
            avatar_url: adminUser.avatar_url,
            last_login_at: adminUser.last_login_at,
            created_at: adminUser.created_at,
          },
        ],
        pagination,
      }) satisfies OpenApiAdminUserListResponse
      return HttpResponse.json(response)
    }),
    http.get('*/api/v1/admin/users/:id', async () => {
      const scenarioResponse = await applyScenario(options)
      if (scenarioResponse) return scenarioResponse
      return HttpResponse.json(ok(adminUser))
    }),
    http.post('*/api/v1/admin/users', async ({ request }) => {
      const body = (await request.json()) as Partial<OpenApiUserDetail>
      Object.assign(adminUser, body)
      return HttpResponse.json(ok(adminUser), { status: 201 })
    }),
    http.patch('*/api/v1/admin/users/:id', async ({ request }) => {
      Object.assign(adminUser, (await request.json()) as Partial<OpenApiUserDetail>)
      return HttpResponse.json(ok(adminUser))
    }),
    http.delete('*/api/v1/admin/users/:id', () => new HttpResponse(null, { status: 204 })),
    http.patch('*/api/v1/admin/users/:id/status', async ({ request }) => {
      const body = (await request.json()) as { status: string }
      adminUser.status = body.status
      return HttpResponse.json(ok({ id: adminUser.id, status: adminUser.status }))
    }),
    http.post('*/api/v1/admin/users/:id/reset-password', async ({ request }) => {
      const body = (await request.json()) as { new_password?: string }
      if (!body.new_password) return HttpResponse.json({ message: 'new_password is required' }, { status: 400 })
      return HttpResponse.json(ok({ message: 'Password reset' }))
    }),
    http.get('*/api/v1/admin/users/:id/audit-logs', () => HttpResponse.json(ok({ items: [], pagination }))),
    http.get('*/api/v1/admin/users/:id/login-logs', () => HttpResponse.json(ok({ items: [], pagination }))),
    http.get('*/api/v1/admin/users/:id/security-summary', () =>
      HttpResponse.json(
        ok({
          user_id: currentUser.id,
          two_factor_enabled: false,
          active_session_count: 1,
          device_count: 1,
          failed_logins_last_30_days: 0,
        })
      )
    ),
    http.get('*/api/v1/admin/users/:id/sessions', () => HttpResponse.json(ok({ items: [], pagination }))),
    http.delete('*/api/v1/admin/users/:id/sessions/:sessionId', () => new HttpResponse(null, { status: 204 })),
    http.get('*/api/v1/admin/roles', () => {
      const response = ok({ items: [role], pagination }) satisfies OpenApiRoleListResponse
      return HttpResponse.json(response)
    }),
    http.get('*/api/v1/admin/roles/:id', () => HttpResponse.json(ok(role))),
    http.post('*/api/v1/admin/roles', () => HttpResponse.json(ok(role), { status: 201 })),
    http.put('*/api/v1/admin/roles/:id', async ({ request }) => {
      Object.assign(role, (await request.json()) as Partial<OpenApiRole>)
      return HttpResponse.json(ok(role))
    }),
    http.delete('*/api/v1/admin/roles/:id', () => new HttpResponse(null, { status: 204 })),
    http.all(/^https?:\/\/[^/]+\/api(?:\/|$)/, ({ request }) =>
      HttpResponse.json(
        { code: 'MOCK_ROUTE_NOT_IMPLEMENTED', message: `No mock handler for ${request.method} ${request.url}` },
        { status: 501 }
      )
    ),
  ]
}
