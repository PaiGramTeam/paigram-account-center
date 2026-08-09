import { expect, test } from '@playwright/test'

test('uses the browser mock to complete administrator sign-in', async ({ page }) => {
  const usersResponse = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/admin/users')

  await page.goto('/login')
  await page.getByPlaceholder('请输入邮箱').fill('admin@example.test')
  await page.getByPlaceholder('请输入密码').fill('mock-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()

  const response = await usersResponse
  expect(response.ok()).toBe(true)
  await expect(response.json()).resolves.toMatchObject({
    data: { items: [{ display_name: 'Mock User' }] },
  })
  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByText('用户增长趋势')).toBeVisible()
  await expect(page.getByText('Mock User')).toBeVisible()

  await page.goto('/users')
  await expect(page.getByText('Mock User')).toBeVisible()

  const resetResponse = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/v1/admin/users/1/reset-password'
  )
  await page.getByRole('button', { name: '更多' }).click()
  await page.getByText('重置密码', { exact: true }).click()
  const resetDialogTitle = page.getByText('设置临时密码', { exact: true })
  await expect(resetDialogTitle).toBeVisible()
  await page.getByPlaceholder('至少 8 位').fill('mock-temporary-password')
  await page.getByPlaceholder('再次输入临时密码').fill('mock-temporary-password')
  await page.getByRole('button', { name: '确定' }).click()
  expect((await resetResponse).ok()).toBe(true)
  await expect(resetDialogTitle).toBeHidden()

  await page.goto('/users/roles')
  await expect(page.getByText('Mock Administrator')).toBeVisible()

  await page.goto('/users/permissions')
  await expect(page.getByText('user:read')).toBeVisible()
})
