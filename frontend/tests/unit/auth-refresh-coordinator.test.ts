import { describe, expect, test } from 'bun:test'
import { AuthRefreshCoordinator } from '../../packages/shared-components/src/api/auth-refresh-coordinator'

describe('AuthRefreshCoordinator', () => {
  test('shares one refresh across concurrent requests', async () => {
    const coordinator = new AuthRefreshCoordinator<string>()
    let calls = 0
    let release: ((value: string) => void) | undefined
    const refresh = () => {
      calls += 1
      return new Promise<string>((resolve) => {
        release = resolve
      })
    }

    const requests = Array.from({ length: 10 }, () => coordinator.run(refresh, () => {}))
    release?.('new-token')

    expect(await Promise.all(requests)).toEqual(Array(10).fill('new-token'))
    expect(calls).toBe(1)
  })

  test('notifies once when a shared refresh fails', async () => {
    const coordinator = new AuthRefreshCoordinator<string>()
    let failures = 0
    let rejectRefresh: ((reason: Error) => void) | undefined
    const refresh = () =>
      new Promise<string>((_resolve, reject) => {
        rejectRefresh = reject
      })

    const requests = Array.from({ length: 4 }, () => coordinator.run(refresh, () => failures++))
    rejectRefresh?.(new Error('expired'))

    const results = await Promise.allSettled(requests)
    expect(results.every((result) => result.status === 'rejected')).toBe(true)
    expect(failures).toBe(1)
  })

  test('waits for terminal session cleanup before rejecting callers', async () => {
    const coordinator = new AuthRefreshCoordinator<string>()
    const releaseCleanup = Promise.withResolvers<void>()
    let cleanupStarted = false

    const request = coordinator.run(
      async () => {
        throw new Error('expired')
      },
      async () => {
        cleanupStarted = true
        await releaseCleanup.promise
      }
    )
    await Promise.resolve()

    expect(cleanupStarted).toBe(true)
    const settledBeforeCleanup = await Promise.race([
      request.then(
        () => true,
        () => true
      ),
      new Promise<boolean>((resolve) => setTimeout(() => resolve(false), 0)),
    ])
    expect(settledBeforeCleanup).toBe(false)

    releaseCleanup.resolve()
    await expect(request).rejects.toThrow('expired')
  })
})
