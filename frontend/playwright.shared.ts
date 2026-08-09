const DEFAULT_SERVER_TIMEOUT = 120_000

export const playwrightPorts = {
  user: Number(process.env.PLAYWRIGHT_USER_PORT ?? 5193),
  admin: Number(process.env.PLAYWRIGHT_ADMIN_PORT ?? 5194),
  userMock: Number(process.env.PLAYWRIGHT_USER_MOCK_PORT ?? 5195),
  adminMock: Number(process.env.PLAYWRIGHT_ADMIN_MOCK_PORT ?? 5196),
}

interface AppProjectOptions {
  name: string
  testDir: string
  port: number
  fullyParallel?: boolean
}

export function appProject({ name, testDir, port, fullyParallel }: AppProjectOptions) {
  return {
    name,
    testDir,
    fullyParallel,
    use: { baseURL: `http://127.0.0.1:${port}` },
  }
}

interface AppServerOptions {
  name: string
  command: string
  port: number
}

export function appServer({ name, command, port }: AppServerOptions) {
  return {
    name,
    command: `${command} -- --host 127.0.0.1 --port ${port}`,
    url: `http://127.0.0.1:${port}/src/pages/login/index.vue`,
    env: { VITE_API_BASE_URL: '/api/v1' },
    reuseExistingServer: !process.env.CI && process.env.PLAYWRIGHT_REUSE_SERVER === '1',
    stderr: process.env.PLAYWRIGHT_SERVER_LOGS === '1' ? ('pipe' as const) : ('ignore' as const),
    timeout: DEFAULT_SERVER_TIMEOUT,
  }
}
