import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

export interface RealSystemState {
  frontend_url: string
  admin_email: string
  admin_password: string
  user_email: string
  user_password: string
}

const testDirectory = path.dirname(fileURLToPath(import.meta.url))
export const frontendRoot = path.resolve(testDirectory, '../..')
export const repositoryRoot = path.resolve(frontendRoot, '..')
export const stateFile = path.join(frontendRoot, 'test-results', 'e2e-real-system.json')
export const systemLogFile = path.join(frontendRoot, 'test-results', 'e2e-real-system.log')

export async function readSystemState(): Promise<RealSystemState> {
  const state = JSON.parse(await readFile(stateFile, 'utf8')) as Partial<RealSystemState>
  for (const key of ['frontend_url', 'admin_email', 'admin_password', 'user_email', 'user_password'] as const) {
    if (!state[key]) throw new Error(`Real-system state is missing ${key}`)
  }
  return state as RealSystemState
}
