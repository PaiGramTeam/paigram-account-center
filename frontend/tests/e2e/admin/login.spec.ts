import { expect, test } from '@playwright/test'

const now = '2026-08-09T00:00:00Z'
const admin = {
  id: 1,
  display_name: 'Routed Administrator',
  primary_email: 'admin@example.test',
  status: 'active',
  created_at: now,
  updated_at: now,
  roles: ['admin'],
  permissions: ['user:read'],
}

test('signs in to the administration dashboard through routed responses', async ({ page }) => {
  await page.route('**/api/v1/auth/login', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 0,
        message: 'OK',
        data: {
          access_token: 'routed-admin-access-token',
          refresh_token: 'routed-admin-refresh-token',
          access_expiry: '2026-08-09T01:00:00Z',
          refresh_expiry: '2026-09-08T00:00:00Z',
          user_id: admin.id,
        },
      }),
    })
  )
  await page.route('**/api/v1/admin/users/1', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 0,
        message: 'OK',
        data: { ...admin, primary_login_type: 'email', emails: [], two_factor_enabled: false },
      }),
    })
  )
  await page.route('**/api/v1/admin/users', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 0,
        message: 'OK',
        data: {
          items: [{ ...admin, primary_login_type: 'email' }],
          pagination: { page: 1, page_size: 20, total: 1, total_pages: 1 },
        },
      }),
    })
  )

  await page.goto('/login')
  await page.getByPlaceholder('请输入邮箱').fill(admin.primary_email)
  await page.getByPlaceholder('请输入密码').fill('correct-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()

  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByText('用户增长趋势')).toBeVisible()
})
