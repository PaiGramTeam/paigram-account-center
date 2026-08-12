import { beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '../../packages/shared-components/src/stores/user'

describe('browser session store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  test('keeps only the access token in memory and advances the session epoch on reset', () => {
    const store = useUserStore()
    const initialEpoch = store.sessionEpoch

    store.setAuthData({ accessToken: 'memory-only-access-token' })

    expect(store.token).toBe('memory-only-access-token')
    expect('refreshToken' in store.$state).toBe(false)

    store.reset()

    expect(store.token).toBe('')
    expect(store.sessionEpoch).toBe(initialEpoch + 1)
  })
})
