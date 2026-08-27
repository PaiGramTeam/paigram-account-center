import { beforeEach, describe, expect, mock, spyOn, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'

const events: string[] = []

const apiModule = () => ({
  authApi: {
    handleOAuthCallback: async () => {
      events.push('callback')
      return { data: { bound: true, purpose: 'bind_login_method' } }
    },
  },
  profileApi: {},
})

mock.module('@/api', apiModule)
mock.module('../../packages/user-app/src/api', apiModule)
mock.module('@arco-design/web-vue', () => ({
  Message: { error: () => undefined, success: () => undefined },
}))
mock.module('@paigram/shared-components', () => ({
  BrowserSessionEndedError: class BrowserSessionEndedError extends Error {},
  browserSessionBroker: {},
  isTerminalBrowserSessionFailure: () => false,
  resolveAuthErrorMessage: (_error: unknown, fallback: string) => fallback,
  useUserStore: () => ({ reset: () => undefined }),
}))

const { useAuthStore } = await import('../../packages/user-app/src/stores/auth')

describe('OAuth binding callback', () => {
  beforeEach(() => {
    events.length = 0
    setActivePinia(createPinia())
  })

  test('restores the browser session before submitting the binding callback', async () => {
    const store = useAuthStore()
    store.bootstrapSession = async () => {
      events.push('bootstrap')
      return true
    }

    const result = await store.handleOAuthCallback('google', { code: 'code', state: 'state' }, 'bind')

    expect(result).toEqual({ status: 'bound' })
    expect(events).toEqual(['bootstrap', 'callback'])
  })

  test('does not submit the binding callback when session restoration fails', async () => {
    const errorLog = spyOn(console, 'error').mockImplementation(() => undefined)
    try {
      const store = useAuthStore()
      store.bootstrapSession = async () => {
        events.push('bootstrap')
        return false
      }

      await expect(store.handleOAuthCallback('google', { code: 'code', state: 'state' }, 'bind')).rejects.toThrow(
        '登录会话已失效'
      )
      expect(events).toEqual(['bootstrap'])
    } finally {
      errorLog.mockRestore()
    }
  })
})
