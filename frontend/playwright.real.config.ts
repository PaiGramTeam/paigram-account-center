import { defineConfig, devices } from '@playwright/test'

const browserName = process.env.PAI_E2E_BROWSER?.trim() || 'chromium'
const browserDevices = {
  chromium: devices['Desktop Chrome'],
  firefox: devices['Desktop Firefox'],
  webkit: devices['Desktop Safari'],
} as const

if (!(browserName in browserDevices)) {
  throw new Error(`Unsupported real-browser project: ${browserName}`)
}

const browserDevice = browserDevices[browserName as keyof typeof browserDevices]

export default defineConfig({
  testDir: './tests/e2e-real',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 90_000,
  expect: { timeout: 15_000 },
  globalSetup: './tests/e2e-real/global-setup.ts',
  outputDir: './test-results/e2e-real',
  reporter: process.env.CI
    ? [['line'], ['junit', { outputFile: process.env.PAI_E2E_JUNIT_PATH || './test-results/e2e-real-junit.xml' }]]
    : 'line',
  projects: [
    {
      name: `${browserName}-real-system`,
      use: {
        ...browserDevice,
        trace: 'retain-on-failure',
        screenshot: 'only-on-failure',
      },
    },
  ],
})
