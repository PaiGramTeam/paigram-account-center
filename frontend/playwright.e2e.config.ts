import { defineConfig } from '@playwright/test'
import { appProject, appServer, playwrightPorts } from './playwright.shared'
import { resolveWorkers } from './playwright.workers'

export default defineConfig({
  forbidOnly: !!process.env.CI,
  workers: resolveWorkers('PLAYWRIGHT_ROUTED_WORKERS', '50%'),
  projects: [
    appProject({
      name: 'user',
      testDir: './tests/e2e/user',
      port: playwrightPorts.user,
    }),
    appProject({
      name: 'admin',
      testDir: './tests/e2e/admin',
      port: playwrightPorts.admin,
    }),
  ],
  webServer: [
    appServer({
      name: 'user',
      command: 'bun run dev:user',
      port: playwrightPorts.user,
    }),
    appServer({
      name: 'admin',
      command: 'bun run dev:admin',
      port: playwrightPorts.admin,
    }),
  ],
})
