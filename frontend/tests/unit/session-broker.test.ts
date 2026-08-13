import { describe, expect, test } from 'bun:test'
import {
  BrowserSessionBroker,
  BrowserSessionCapabilityError,
  BrowserSessionEndedError,
} from '../../packages/shared-components/src/api/session-broker'
import { synchronizeBrowserSession } from '../../packages/shared-components/src/api/browser-session-sync'

class SharedStorage {
  values = new Map<string, string>()
  getItem(key: string): string | null {
    return this.values.get(key) ?? null
  }
  setItem(key: string, value: string): void {
    this.values.set(key, value)
  }
}

class ChannelBus {
  listeners = new Set<(event: { data: unknown }) => void>()
  channel() {
    return {
      postMessage: (data: unknown) => queueMicrotask(() => this.listeners.forEach((listener) => listener({ data }))),
      addEventListener: (_type: 'message', listener: (event: { data: unknown }) => void) =>
        this.listeners.add(listener),
      removeEventListener: (_type: 'message', listener: (event: { data: unknown }) => void) =>
        this.listeners.delete(listener),
    }
  }
}

class LockQueue {
  pending = Promise.resolve()
  request<T>(_name: string, callback: () => Promise<T>): Promise<T> {
    const result = this.pending.then(callback)
    this.pending = result.then(
      () => undefined,
      () => undefined
    )
    return result
  }
}

const firstSource = '00000000-0000-4000-8000-000000000101'
const secondSource = '00000000-0000-4000-8000-000000000102'

describe('cross-tab browser session broker', () => {
  test('rotates a shared refresh cookie only once across concurrent tabs', async () => {
    const storage = new SharedStorage()
    const bus = new ChannelBus()
    const locks = new LockQueue()
    const first = new BrowserSessionBroker({
      source: firstSource,
      storage,
      channel: bus.channel(),
      locks,
      requestTimeoutMs: 25,
    })
    const second = new BrowserSessionBroker({
      source: secondSource,
      storage,
      channel: bus.channel(),
      locks,
      requestTimeoutMs: 25,
    })
    first.establish({ accessToken: 'expired-access-token', userId: 42 })
    const logicalSessionId = first.currentSessionId()
    await Promise.resolve()
    let refreshes = 0
    const refresh = async () => {
      refreshes += 1
      return { accessToken: 'shared-access-token', userId: 42 }
    }

    const sessions = await Promise.all([
      first.refresh(refresh, { rejectedAccessToken: 'expired-access-token' }),
      second.refresh(refresh, { rejectedAccessToken: 'expired-access-token' }),
    ])

    expect(refreshes).toBe(1)
    expect(first.currentSessionId()).toBe(logicalSessionId)
    expect(second.currentSessionId()).toBe(logicalSessionId)
    expect(sessions).toEqual([
      { accessToken: 'shared-access-token', userId: 42 },
      { accessToken: 'shared-access-token', userId: 42 },
    ])
    const marker = [...storage.values.values()][0]
    expect(marker).not.toContain('shared-access-token')
    expect(marker).not.toContain('userId')
    first.dispose()
    second.dispose()
  })

  test('broadcasts logout and refuses silent refresh until an explicit login', async () => {
    const storage = new SharedStorage()
    const bus = new ChannelBus()
    const locks = new LockQueue()
    const first = new BrowserSessionBroker({ source: firstSource, storage, channel: bus.channel(), locks })
    const second = new BrowserSessionBroker({ source: secondSource, storage, channel: bus.channel(), locks })
    const events: string[] = []
    second.subscribe((event) => events.push(event.type))
    first.establish({ accessToken: 'ephemeral-token', userId: 42 })
    await Promise.resolve()

    await first.logout(async () => undefined)
    await Promise.resolve()

    expect(events).toContain('authenticated')
    expect(events.at(-1)).toBe('ended')
    expect(second.isEnded()).toBe(true)
    await expect(second.refresh(async () => ({ accessToken: 'must-not-run', userId: 42 }))).rejects.toBeInstanceOf(
      BrowserSessionEndedError
    )
    const marker = [...storage.values.values()][0]
    expect(marker).not.toContain('ephemeral-token')
    expect(marker).not.toContain('userId')
    first.dispose()
    second.dispose()
  })

  test('replays the current authenticated session to a late subscriber', async () => {
    const storage = new SharedStorage()
    const bus = new ChannelBus()
    const locks = new LockQueue()
    const first = new BrowserSessionBroker({ source: firstSource, storage, channel: bus.channel(), locks })
    const second = new BrowserSessionBroker({ source: secondSource, storage, channel: bus.channel(), locks })

    first.establish({ accessToken: 'current-access-token', userId: 42 })
    await Promise.resolve()

    const events: string[] = []
    second.subscribe((event) => events.push(event.type))
    await Promise.resolve()

    expect(events).toEqual(['authenticated'])
    first.dispose()
    second.dispose()
  })

  test('does not let a rejected stale token end a newer session', async () => {
    const storage = new SharedStorage()
    const bus = new ChannelBus()
    const locks = new LockQueue()
    const first = new BrowserSessionBroker({ source: firstSource, storage, channel: bus.channel(), locks })
    const second = new BrowserSessionBroker({ source: secondSource, storage, channel: bus.channel(), locks })
    first.establish({ accessToken: 'stale-access-token', userId: 42 })
    await Promise.resolve()
    second.establish({ accessToken: 'new-access-token', userId: 42 })
    await Promise.resolve()

    await expect(first.invalidateIfCurrent('stale-access-token')).resolves.toBe(false)
    expect(first.isEnded()).toBe(false)
    expect(first.isSessionCurrent({ accessToken: 'new-access-token', userId: 42 })).toBe(true)
    expect(first.isUserSessionCurrent(42)).toBe(true)
    first.dispose()
    second.dispose()
  })

  test('serializes an explicit login after an in-flight logout', async () => {
    const storage = new SharedStorage()
    const bus = new ChannelBus()
    const locks = new LockQueue()
    const first = new BrowserSessionBroker({ source: firstSource, storage, channel: bus.channel(), locks })
    const second = new BrowserSessionBroker({ source: secondSource, storage, channel: bus.channel(), locks })
    first.establish({ accessToken: 'old-access-token', userId: 42 })
    const oldLogicalSessionId = first.currentSessionId()
    const releaseLogout = Promise.withResolvers<void>()
    const logoutStarted = Promise.withResolvers<void>()
    const logout = first.logout(async () => {
      logoutStarted.resolve()
      await releaseLogout.promise
    })
    await logoutStarted.promise
    let loginStarted = false
    let loginObservedEndedSession = false

    const login = second.authenticate(async () => {
      loginStarted = true
      loginObservedEndedSession = second.isEnded()
      return {
        value: 'logged-in',
        session: { accessToken: 'replacement-access-token', userId: 84 },
      }
    })
    await Promise.resolve()
    expect(loginStarted).toBe(false)

    releaseLogout.resolve()
    await expect(logout).resolves.toBeUndefined()
    await expect(login).resolves.toBe('logged-in')
    expect(loginObservedEndedSession).toBe(true)
    expect(second.currentSessionId()).not.toBe(oldLogicalSessionId)
    expect(second.isEnded()).toBe(false)
    expect(second.isSessionCurrent({ accessToken: 'replacement-access-token', userId: 84 })).toBe(true)
    first.dispose()
    second.dispose()
  })

  test('handles a terminal refresh failure before admitting the next login', async () => {
    const storage = new SharedStorage()
    const bus = new ChannelBus()
    const locks = new LockQueue()
    const first = new BrowserSessionBroker({ source: firstSource, storage, channel: bus.channel(), locks })
    const second = new BrowserSessionBroker({ source: secondSource, storage, channel: bus.channel(), locks })
    const refreshStarted = Promise.withResolvers<void>()
    const rejectRefresh = Promise.withResolvers<never>()
    let failureHandled = false
    let loginObservedFailure = false

    const refresh = first.refresh(
      async () => {
        refreshStarted.resolve()
        return rejectRefresh.promise
      },
      {
        onFailure: () => {
          failureHandled = true
          first.invalidate()
        },
      }
    )
    await refreshStarted.promise
    const login = second.authenticate(async () => {
      loginObservedFailure = failureHandled
      return {
        value: undefined,
        session: { accessToken: 'new-session-after-failure', userId: 42 },
      }
    })

    rejectRefresh.reject(new Error('refresh rejected'))
    await expect(refresh).rejects.toThrow('refresh rejected')
    await expect(login).resolves.toBeUndefined()
    expect(loginObservedFailure).toBe(true)
    expect(second.isSessionCurrent({ accessToken: 'new-session-after-failure', userId: 42 })).toBe(true)
    first.dispose()
    second.dispose()
  })

  test('commits refreshed local state before admitting a queued logout', async () => {
    const storage = new SharedStorage()
    const bus = new ChannelBus()
    const locks = new LockQueue()
    const first = new BrowserSessionBroker({ source: firstSource, storage, channel: bus.channel(), locks })
    const second = new BrowserSessionBroker({ source: secondSource, storage, channel: bus.channel(), locks })
    const refreshStarted = Promise.withResolvers<void>()
    const releaseRefresh = Promise.withResolvers<void>()
    let committed = false
    let logoutObservedCommit = false

    const refresh = first.refresh(
      async () => {
        refreshStarted.resolve()
        await releaseRefresh.promise
        return { accessToken: 'rotated-access-token', userId: 42 }
      },
      {
        commit: () => {
          committed = true
        },
      }
    )
    await refreshStarted.promise
    const logout = second.logout(async () => {
      logoutObservedCommit = committed
    })

    releaseRefresh.resolve()
    await expect(refresh).resolves.toEqual({ accessToken: 'rotated-access-token', userId: 42 })
    await expect(logout).resolves.toBeUndefined()
    expect(logoutObservedCommit).toBe(true)
    first.dispose()
    second.dispose()
  })

  test('only rolls back the exact failed authentication session', async () => {
    const storage = new SharedStorage()
    const bus = new ChannelBus()
    const locks = new LockQueue()
    const first = new BrowserSessionBroker({ source: firstSource, storage, channel: bus.channel(), locks })
    const second = new BrowserSessionBroker({ source: secondSource, storage, channel: bus.channel(), locks })
    const staleSession = { accessToken: 'stale-login-token', userId: 42 }
    first.establish(staleSession)
    await Promise.resolve()
    second.establish({ accessToken: 'current-login-token', userId: 84 })
    await Promise.resolve()
    let remoteLogoutCalled = false

    await expect(
      first.logoutIfCurrent(staleSession, async () => {
        remoteLogoutCalled = true
      })
    ).resolves.toBe(false)

    expect(remoteLogoutCalled).toBe(false)
    expect(first.isSessionCurrent({ accessToken: 'current-login-token', userId: 84 })).toBe(true)
    first.dispose()
    second.dispose()
  })

  test('fails closed when secure cross-tab coordination is unavailable', async () => {
    const broker = new BrowserSessionBroker({ source: firstSource, storage: new SharedStorage() })

    expect(() => broker.establish({ accessToken: 'unsafe-token', userId: 42 })).toThrow(BrowserSessionCapabilityError)
    await expect(broker.refresh(async () => ({ accessToken: 'unsafe-token', userId: 42 }))).rejects.toBeInstanceOf(
      BrowserSessionCapabilityError
    )
    await expect(
      broker.authenticate(async () => ({ value: undefined, session: { accessToken: 'unsafe-token', userId: 42 } }))
    ).rejects.toBeInstanceOf(BrowserSessionCapabilityError)
    broker.dispose()
  })

  test('still revokes the remote session when coordination capabilities are unavailable', async () => {
    const broker = new BrowserSessionBroker({ source: firstSource, storage: new SharedStorage() })
    let revoked = false

    await expect(
      broker.logout(async () => {
        revoked = true
      })
    ).resolves.toBeUndefined()

    expect(revoked).toBe(true)
    expect(broker.isEnded()).toBe(true)
    broker.dispose()
  })

  test('drops a superseded principal event while its safe-route navigation is pending', async () => {
    const storage = new SharedStorage()
    const bus = new ChannelBus()
    const locks = new LockQueue()
    const publisher = new BrowserSessionBroker({ source: firstSource, storage, channel: bus.channel(), locks })
    const subscriber = new BrowserSessionBroker({ source: secondSource, storage, channel: bus.channel(), locks })
    const navigationStarted = Promise.withResolvers<void>()
    const releaseNavigation = Promise.withResolvers<void>()
    const accessTokens: string[] = []
    const loadedProfiles: number[] = []
    let state = { accessToken: 'original-token', authenticated: true, profileUserId: 1 as number | null }
    const unsubscribe = synchronizeBrowserSession(
      {
        readState: () => state,
        clearSession: () => {
          state = { accessToken: '', authenticated: false, profileUserId: null }
        },
        setAccessToken: (accessToken) => {
          accessTokens.push(accessToken)
          state = { ...state, accessToken, authenticated: true }
        },
        loadProfile: async (userId) => {
          loadedProfiles.push(userId)
          state = { ...state, profileUserId: userId }
        },
        navigateBeforePrincipalChange: async () => {
          navigationStarted.resolve()
          await releaseNavigation.promise
        },
        navigateToLogin: async () => undefined,
        navigateAfterAuthentication: async () => undefined,
      },
      subscriber
    )

    publisher.establish({ accessToken: 'superseded-token', userId: 2 })
    await navigationStarted.promise
    publisher.establish({ accessToken: 'current-token', userId: 3 })
    await Promise.resolve()
    await Promise.resolve()
    releaseNavigation.resolve()
    await Promise.resolve()

    expect(accessTokens).toEqual(['current-token'])
    expect(loadedProfiles).toEqual([3])
    expect(state).toEqual({ accessToken: 'current-token', authenticated: true, profileUserId: 3 })
    unsubscribe()
    publisher.dispose()
    subscriber.dispose()
  })
})
