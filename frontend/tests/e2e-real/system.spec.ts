import { expect, test, type Request } from '@playwright/test'
import { readSystemState } from './system-state'

test.describe.serial('production browser journeys', () => {
  test('user registers, signs in, binds Mihomo, reads profiles, and logs out', async ({
    page,
    context,
    browserName,
  }) => {
    const system = await readSystemState()
    await page.goto(`${system.frontend_url}/register`)
    await page.getByPlaceholder('请输入显示名称').fill('Browser User')
    await page.getByPlaceholder('请输入邮箱地址').fill(system.user_email)
    await page.getByPlaceholder('请输入密码（8-72个字符）').fill(system.user_password)
    await page.getByPlaceholder('请再次输入密码').fill(system.user_password)
    await page.getByRole('button', { name: '注册', exact: true }).click()

    await expect(page.getByRole('heading', { name: '注册成功！' })).toBeVisible()
    await page.getByRole('button', { name: '前往登录' }).click()
    await expect(page).toHaveURL(/\/login$/)
    await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
    await page.getByPlaceholder('请输入邮箱地址').fill(system.user_email)
    await page.getByPlaceholder('请输入密码').fill(system.user_password)
    await page.getByRole('button', { name: '登录', exact: true }).click()

    await expect(page).toHaveURL(/\/dashboard$/)
    await expect(page.getByRole('heading', { name: '欢迎回来，Browser User' })).toBeVisible()
    await page.goto(`${system.frontend_url}/platform-accounts`)
    await page.getByRole('button', { name: '绑定平台账号' }).click()
    const createDialog = page.locator('.arco-modal:visible')
    await createDialog.getByPlaceholder('请选择平台').click()
    await page.getByText('Mihomo', { exact: true }).click()
    await createDialog.getByPlaceholder('例如：我的主账号').fill('E2E Mihomo')
    await createDialog.getByPlaceholder('例如：{"cookie_token":"..."}').fill(
      JSON.stringify({
        cookie_bundle: JSON.stringify({ cookie_token: 'browser-e2e' }),
        device_id: 'browser-device',
        device_fp: 'browser-fingerprint',
        device_name: 'Browser E2E',
      })
    )
    await createDialog.getByRole('button', { name: '确定', exact: true }).click()

    await expect(page.getByRole('cell', { name: 'E2E Mihomo' })).toBeVisible()
    await expect(page.getByRole('cell', { name: 'active' })).toBeVisible()
    await page.getByRole('button', { name: '管理', exact: true }).click()
    const accountDrawer = page.locator('.arco-drawer:visible')
    await expect(accountDrawer.getByText('Traveler', { exact: true })).toBeVisible()
    await expect(accountDrawer.getByText('Aether', { exact: true })).toBeVisible()
    await accountDrawer.getByRole('button', { name: 'Close' }).click()

    await page.close()
    const firstTab = await context.newPage()
    const secondTab = await context.newPage()
    let refreshRequests = 0
    const sessionLockEvents: Array<{ tab: string; ifAvailable: boolean }> = []
    await context.exposeBinding('recordSessionLockRequest', ({ page: sourcePage }, ifAvailable: boolean) => {
      const tab = sourcePage === firstTab ? 'first' : sourcePage === secondTab ? 'second' : 'unexpected'
      sessionLockEvents.push({ tab, ifAvailable })
    })
    await context.exposeBinding(
      'sessionLockContendersReady',
      () => new Set(sessionLockEvents.filter((event) => event.ifAvailable).map((event) => event.tab)).size >= 2
    )
    const enforceLockBarrier = browserName !== 'webkit'
    await context.addInitScript(`
      {
        const enforceLockBarrier = ${enforceLockBarrier}
        const originalRequest = navigator.locks.request.bind(navigator.locks)
        navigator.locks.request = function (...args) {
          const callbackIndex = typeof args[1] === 'function' ? 1 : 2
          const callback = args[callbackIndex]
          const recorded = globalThis.recordSessionLockRequest(Boolean(args[1] && args[1].ifAvailable))
          args[callbackIndex] = async (...callbackArgs) => {
            await recorded
            if (enforceLockBarrier) {
              while (!(await globalThis.sessionLockContendersReady())) {
                await new Promise((resolve) => setTimeout(resolve, 10))
              }
            }
            return callback(...callbackArgs)
          }
          return originalRequest(...args)
        }
      }
    `)
    const countRefresh = (request: Request) => {
      if (new URL(request.url()).pathname === '/api/v1/auth/refresh' && request.method() === 'POST') {
        refreshRequests += 1
      }
    }
    context.on('request', countRefresh)
    await Promise.all([
      firstTab.goto(`${system.frontend_url}/dashboard`),
      secondTab.goto(`${system.frontend_url}/dashboard`),
    ])
    await Promise.all([
      expect(firstTab.getByRole('heading', { name: '欢迎回来，Browser User' })).toBeVisible({ timeout: 30_000 }),
      expect(secondTab.getByRole('heading', { name: '欢迎回来，Browser User' })).toBeVisible({ timeout: 30_000 }),
    ])
    expect(refreshRequests).toBe(1)
    const conditionalLockTabs = new Set(
      sessionLockEvents.filter((event) => event.ifAvailable).map((event) => event.tab)
    )
    if (enforceLockBarrier) {
      expect(conditionalLockTabs).toEqual(new Set(['first', 'second']))
      expect(sessionLockEvents.some((event) => !event.ifAvailable)).toBe(true)
    } else {
      expect(conditionalLockTabs.size).toBeGreaterThanOrEqual(1)
    }
    context.off('request', countRefresh)

    await firstTab.locator('.arco-layout-header .arco-avatar').click()
    await firstTab.getByText('退出登录', { exact: true }).click()
    await expect(firstTab).toHaveURL(/\/login$/)
    await expect(secondTab).toHaveURL((url) => url.pathname === '/login')
    await secondTab.close()
  })

  test('administrator reads the user platform binding through the admin application', async ({ page }) => {
    const system = await readSystemState()
    await page.goto(`${system.frontend_url}/admin/login`)
    await page.getByPlaceholder('请输入邮箱').fill(system.admin_email)
    await page.getByPlaceholder('请输入密码').fill(system.admin_password)
    await page.getByRole('button', { name: '登录', exact: true }).click()

    await expect(page).toHaveURL(/\/admin\/platform-accounts$/)
    await expect(page.getByRole('cell', { name: 'E2E Mihomo' })).toBeVisible()
    await page.getByRole('button', { name: '查看', exact: true }).click()
    await expect(page.getByText('Traveler', { exact: true })).toBeVisible()
    await expect(page.getByText('Aether', { exact: true })).toBeVisible()
  })
})
