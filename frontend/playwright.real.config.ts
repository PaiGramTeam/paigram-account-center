import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests/e2e-real',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  timeout: 90_000,
  expect: { timeout: 15_000 },
  globalSetup: './tests/e2e-real/global-setup.ts',
  outputDir: './test-results/e2e-real',
  reporter: process.env.CI
    ? [['line'], ['junit', { outputFile: process.env.PAI_E2E_JUNIT_PATH || './test-results/e2e-real-junit.xml' }]]
    : 'line',
  projects: [
    {
      name: 'chromium-real-system',
      use: {
        ...devices['Desktop Chrome'],
        trace: 'retain-on-failure',
        screenshot: 'only-on-failure',
      },
    },
  ],
})
