import type { FullConfig } from '@playwright/test'
import { execFileSync, spawn, spawnSync, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { createWriteStream } from 'node:fs'
import { copyFile, cp, mkdir, readFile, rm } from 'node:fs/promises'
import path from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { repositoryRoot, stateFile, systemLogFile, type RealSystemState } from './system-state'

const startupTimeoutMilliseconds = 10 * 60 * 1000

export default async function globalSetup(_config: FullConfig): Promise<() => Promise<void>> {
  await mkdir(path.dirname(stateFile), { recursive: true })
  await Promise.all([rm(stateFile, { force: true }), rm(systemLogFile, { force: true })])

  const provider = resolveProvider()
  const containerCLI = process.env.PAI_E2E_CONTAINER_CLI?.trim() || provider
  const image = `paigram-e2e-frontend:${process.pid}`
  let systemProcess: ChildProcessWithoutNullStreams | undefined
  let imageBuilt = false
  try {
    await buildFrontendImage(containerCLI, image)
    imageBuilt = true
    const logStream = createWriteStream(systemLogFile, { flags: 'a', mode: 0o600 })
    systemProcess = spawn(
      'go',
      [
        'run',
        '-tags=integration',
        './cmd/e2e-real-system',
        '--repository-root',
        repositoryRoot,
        '--state-file',
        stateFile,
        '--frontend-image',
        image,
      ],
      {
        cwd: path.join(repositoryRoot, 'services', 'account-center'),
        env: { ...process.env, PAI_TESTCONTAINERS_PROVIDER: provider },
        stdio: ['pipe', 'pipe', 'pipe'],
      }
    )
    systemProcess.stdout.pipe(logStream, { end: false })
    systemProcess.stderr.pipe(logStream, { end: false })
    systemProcess.once('exit', () => logStream.end())
    await waitForSystem(systemProcess)
  } catch (error) {
    if (systemProcess) await stopSystem(systemProcess)
    if (imageBuilt) removeImage(containerCLI, image)
    throw error
  }

  return async () => {
    if (systemProcess) await stopSystem(systemProcess)
    removeImage(containerCLI, image)
  }
}

async function buildFrontendImage(containerCLI: string, image: string): Promise<void> {
  const sourceCommit = gitOutput(['rev-parse', 'HEAD'])
  const contractCommits = gitOutput([
    'log',
    '--diff-filter=A',
    '--format=%H',
    '--',
    'contracts/proto/platform/v2/types.proto',
  ])
    .split(/\r?\n/)
    .filter(Boolean)
  const contractBaseline = contractCommits.at(-1)
  if (!contractBaseline) throw new Error('Unable to resolve the v2 contract baseline')
  const pyproject = execFileSync('git', ['show', 'HEAD:sdks/python/pyproject.toml'], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  })
  const sdkVersion = /^version\s*=\s*"([^"]+)"/m.exec(pyproject)?.[1]
  if (!sdkVersion) throw new Error('Unable to resolve the Python SDK version')

  const useLocalDist = process.env.PAI_E2E_LOCAL_DIST === '1'
  let buildContext = repositoryRoot
  let containerfile = path.join(repositoryRoot, 'deploy', 'podman', 'frontend.Containerfile')
  if (useLocalDist) {
    const frontendEnvironment = { VITE_API_BASE_URL: '/api/v1' }
    runChecked('bun', ['run', '--cwd', frontendRoot(), 'build:user'], frontendEnvironment)
    runChecked(
      'bun',
      ['run', '--cwd', path.join(frontendRoot(), 'packages', 'admin-app'), 'build', '--', '--base=/admin/'],
      frontendEnvironment
    )
    buildContext = path.join(frontendRoot(), 'test-results', 'e2e-real-runtime-context')
    await rm(buildContext, { recursive: true, force: true })
    await mkdir(buildContext, { recursive: true })
    await Promise.all([
      cp(path.join(frontendRoot(), 'packages', 'user-app', 'dist'), path.join(buildContext, 'user'), {
        recursive: true,
      }),
      cp(path.join(frontendRoot(), 'packages', 'admin-app', 'dist'), path.join(buildContext, 'admin'), {
        recursive: true,
      }),
      copyFile(path.join(repositoryRoot, 'deploy', 'podman', 'nginx.conf'), path.join(buildContext, 'nginx.conf')),
      copyFile(
        path.join(frontendRoot(), 'tests', 'e2e-real', 'runtime.Containerfile'),
        path.join(buildContext, 'Containerfile')
      ),
    ])
    containerfile = path.join(buildContext, 'Containerfile')
  }

  runChecked(containerCLI, [
    'build',
    '--file',
    containerfile,
    '--build-arg',
    `VCS_REF=${sourceCommit}`,
    '--build-arg',
    `CONTRACT_BASELINE=${contractBaseline}`,
    '--build-arg',
    `SDK_VERSION=${sdkVersion}`,
    '--tag',
    image,
    buildContext,
  ])
  if (useLocalDist) await rm(buildContext, { recursive: true, force: true })
}

async function waitForSystem(systemProcess: ChildProcessWithoutNullStreams): Promise<RealSystemState> {
  const deadline = Date.now() + startupTimeoutMilliseconds
  while (Date.now() < deadline) {
    if (systemProcess.exitCode !== null) {
      throw new Error(`Real-system fixture exited with code ${systemProcess.exitCode}; see ${systemLogFile}`)
    }
    try {
      const state = JSON.parse(await readFile(stateFile, 'utf8')) as RealSystemState
      if (state.frontend_url) return state
    } catch (error) {
      const code = (error as NodeJS.ErrnoException).code
      if (code !== 'ENOENT' && !(error instanceof SyntaxError)) throw error
    }
    await delay(250)
  }
  throw new Error(`Real-system fixture did not become ready; see ${systemLogFile}`)
}

async function stopSystem(systemProcess: ChildProcessWithoutNullStreams): Promise<void> {
  if (systemProcess.exitCode !== null) return
  systemProcess.stdin.end('shutdown\n')
  const exited = new Promise<void>((resolve) => systemProcess.once('exit', () => resolve()))
  await Promise.race([exited, delay(60_000)])
  if (systemProcess.exitCode === null) {
    systemProcess.kill()
    await Promise.race([exited, delay(10_000)])
  }
}

function removeImage(containerCLI: string, image: string): void {
  spawnSync(containerCLI, ['image', 'rm', '--force', image], { cwd: repositoryRoot, stdio: 'ignore' })
}

function runChecked(command: string, args: string[], environment?: NodeJS.ProcessEnv): void {
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    env: { ...process.env, ...environment },
    stdio: 'inherit',
  })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`${command} exited with code ${result.status}`)
}

function gitOutput(args: string[]): string {
  return execFileSync('git', args, { cwd: repositoryRoot, encoding: 'utf8' }).trim()
}

function frontendRoot(): string {
  return path.join(repositoryRoot, 'frontend')
}

function resolveProvider(): 'docker' | 'podman' {
  const configured = process.env.PAI_TESTCONTAINERS_PROVIDER?.trim().toLowerCase()
  if (configured === 'docker' || configured === 'podman') return configured
  const cli = process.env.PAI_E2E_CONTAINER_CLI?.trim().toLowerCase() || ''
  return cli.includes('docker') ? 'docker' : 'podman'
}
