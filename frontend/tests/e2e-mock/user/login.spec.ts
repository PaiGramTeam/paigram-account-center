import { expect, test } from '@playwright/test'

test('uses the browser mock to complete user sign-in', async ({ page }) => {
  await page.goto('/login')
  await page.getByPlaceholder('请输入邮箱地址').fill('mock@example.test')
  await page.getByPlaceholder('请输入密码').fill('mock-password')
  await page.getByRole('button', { name: '登录', exact: true }).click()

  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByRole('heading', { name: '欢迎回来，Mock User' })).toBeVisible()

  await page.goto('/account/info')
  await expect(page.getByRole('textbox', { name: '显示名称' })).toHaveValue('Mock User')
  await expect(page.locator('input[disabled]')).toHaveValue('mock@example.test')

  await page.goto('/account/logs')
  await expect(page.getByRole('heading', { name: '账号活动' })).toBeVisible()

  await page.goto('/account/security')
  await page.getByRole('button', { name: '启用 2FA' }).click()
  const twoFactorModal = page.locator('.arco-modal').filter({ hasText: '启用双因素认证' })
  await expect(twoFactorModal).toBeVisible()
  await twoFactorModal.getByPlaceholder('请输入当前密码').fill('mock-password')
  await twoFactorModal.getByRole('button', { name: '继续' }).click()
  await expect(twoFactorModal.getByText('MOCKTOTPSECRET')).toBeVisible()
})
