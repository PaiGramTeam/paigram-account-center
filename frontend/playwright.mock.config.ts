import { defineConfig } from '@playwright/test'
import { appProject, appServer, playwrightPorts } from './playwright.shared'
import { resolveWorkers } from './playwright.workers'

export default defineConfig({
  forbidOnly: !!process.env.CI,
  fullyParallel: true,
  workers: resolveWorkers('PLAYWRIGHT_MOCK_WORKERS', '50%'),
  projects: [
    appProject({
      name: 'user-mock',
      testDir: './tests/e2e-mock/user',
      port: playwrightPorts.userMock,
    }),
    appProject({
      name: 'admin-mock',
      testDir: './tests/e2e-mock/admin',
      port: playwrightPorts.adminMock,
    }),
  ],
  webServer: [
    appServer({
      name: 'user-mock',
      command: 'bun run dev:user:mock',
      port: playwrightPorts.userMock,
    }),
    appServer({
      name: 'admin-mock',
      command: 'bun run dev:admin:mock',
      port: playwrightPorts.adminMock,
    }),
  ],
})
