import { expect, test } from '@playwright/test'

const now = '2026-08-09T00:00:00Z'

test('signs in through the routed API contract', async ({ page }) => {
  await page.route('**/api/v1/auth/login', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 0,
        message: 'OK',
        data: {
          access_token: 'routed-access-token',
          access_expiry: '2026-08-09T01:00:00Z',
          refresh_expiry: '2026-09-08T00:00:00Z',
          user_id: 1,
        },
      }),
    })
  )
  await page.route('**/api/v1/me', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        code: 0,
        message: 'OK',
        data: {
          id: 1,
          display_name: 'Routed User',
          primary_email: 'routed@example.test',
          emails: [],
          login_methods: [],
          status: 'active',
          created_at: now,
          updated_at: now,
        },
      }),
    })
  )

  await page.goto('/login')
  await page.getByPlaceholder('请输入邮箱地址').fill('routed@example.test')
  await page.getByPlaceholder('请输入密码').fill('correct-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()

  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByRole('heading', { name: '欢迎回来，Routed User' })).toBeVisible()
})
