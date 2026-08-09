import { defineConfig } from '@playwright/test'
import { appProject, appServer, playwrightPorts } from './playwright.shared'
import { resolveWorkers } from './playwright.workers'

export default defineConfig({
  forbidOnly: !!process.env.CI,
  workers: resolveWorkers('PLAYWRIGHT_WORKERS', '50%'),
  projects: [
    appProject({
      name: 'user-routed',
      testDir: './tests/e2e/user',
      port: playwrightPorts.user,
    }),
    appProject({
      name: 'admin-routed',
      testDir: './tests/e2e/admin',
      port: playwrightPorts.admin,
    }),
    appProject({
      name: 'user-mock',
      testDir: './tests/e2e-mock/user',
      fullyParallel: true,
      port: playwrightPorts.userMock,
    }),
    appProject({
      name: 'admin-mock',
      testDir: './tests/e2e-mock/admin',
      fullyParallel: true,
      port: playwrightPorts.adminMock,
    }),
  ],
  webServer: [
    appServer({
      name: 'user-routed',
      command: 'bun run dev:user',
      port: playwrightPorts.user,
    }),
    appServer({
      name: 'admin-routed',
      command: 'bun run dev:admin',
      port: playwrightPorts.admin,
    }),
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
