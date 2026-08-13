import { describe, expect, test } from 'bun:test'
import { createRequest, normalizeApiError } from '../../packages/shared-components/src/api/request'

describe('normalizeApiError', () => {
  test('returns the safe fallback for an empty response', () => {
    expect(normalizeApiError(undefined)).toEqual({ message: '请求失败' })
  })

  test('preserves a structured backend error', () => {
    expect(
      normalizeApiError({
        code: 'BINDING_NOT_FOUND',
        error: {
          code: 'BINDING_NOT_FOUND',
          message: 'binding not found',
          details: { binding_id: 42 },
        },
      })
    ).toEqual({
      code: 'BINDING_NOT_FOUND',
      error: 'binding not found',
      message: 'binding not found',
      details: { binding_id: 42 },
    })
  })
})

describe('authenticated request recovery', () => {
  test('does not attempt refresh for authentication endpoint failures', async () => {
    let refreshAttempts = 0
    const request = createRequest({
      setAuthData: () => undefined,
      coordinateRefresh: async () => {
        refreshAttempts += 1
        return { accessToken: 'unexpected-token', userId: 42 }
      },
    })
    request.instance.defaults.adapter = async (config) =>
      Promise.reject({
        config,
        response: { status: 401, data: { message: 'invalid credentials' } },
      })

    await expect(request.post('/auth/login', {}, { skipErrorToast: true })).rejects.toMatchObject({ status: 401 })
    expect(refreshAttempts).toBe(0)
  })

  test('reports the access token rejected by the retried request', async () => {
    let accessToken = 'old-access-token'
    const rejectedTokens: string[] = []
    const request = createRequest({
      getToken: () => accessToken,
      setAuthData: (data) => {
        accessToken = data.accessToken
      },
      coordinateRefresh: async (_refresh, options) => {
        const session = { accessToken: 'new-access-token', userId: 42 }
        options.commit(session)
        return session
      },
      onUnauthorized: (rejectedAccessToken) => {
        rejectedTokens.push(rejectedAccessToken)
      },
    })
    request.instance.defaults.adapter = async (config) =>
      Promise.reject({
        config,
        response: { status: 401, data: { message: 'expired' } },
      })

    await expect(request.get('/me', { skipErrorToast: true })).rejects.toMatchObject({ status: 401 })
    expect(rejectedTokens).toEqual(['new-access-token'])
  })

  test('never replays an old principals request after an explicit session change', async () => {
    let accessToken = 'principal-a-token'
    let sessionId = 'principal-a-session'
    let adapterCalls = 0
    let refreshAttempts = 0
    const request = createRequest({
      getToken: () => accessToken,
      getSessionId: () => sessionId,
      setAuthData: (data) => {
        accessToken = data.accessToken
      },
      coordinateRefresh: async () => {
        refreshAttempts += 1
        return { accessToken: 'must-not-be-used', userId: 42 }
      },
    })
    request.instance.defaults.adapter = async (config) => {
      adapterCalls += 1
      sessionId = 'principal-b-session'
      accessToken = 'principal-b-token'
      return Promise.reject({
        config,
        response: { status: 401, data: { message: 'old session expired' } },
      })
    }

    await expect(request.post('/me/mutation', {}, { skipErrorToast: true })).rejects.toMatchObject({
      name: 'BrowserSessionChangedError',
    })
    expect(adapterCalls).toBe(1)
    expect(refreshAttempts).toBe(0)
    expect(accessToken).toBe('principal-b-token')
  })

  test('rejects an old successful response after an explicit session change', async () => {
    let sessionId = 'principal-a-session'
    const request = createRequest({
      getToken: () => 'principal-a-token',
      getSessionId: () => sessionId,
    })
    request.instance.defaults.adapter = async (config) => {
      sessionId = 'principal-b-session'
      return {
        config,
        data: { data: { private_value: 'principal-a-data' } },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }

    await expect(request.get('/me/private', { skipErrorToast: true })).rejects.toMatchObject({
      name: 'BrowserSessionChangedError',
    })
  })

  test('checks the logical session again before dispatching a retried mutation', async () => {
    let accessToken = 'principal-a-token'
    let sessionId = 'principal-a-session'
    let adapterCalls = 0
    const request = createRequest({
      getToken: () => accessToken,
      getSessionId: () => sessionId,
      setAuthData: (data) => {
        accessToken = data.accessToken
      },
      coordinateRefresh: async (_refresh, options) => {
        const session = { accessToken: 'principal-a-refreshed-token', userId: 42 }
        options.commit(session)
        return session
      },
    })
    request.instance.interceptors.request.use((config) => {
      const retried = config as typeof config & { authRetryAttempted?: boolean }
      if (retried.authRetryAttempted) {
        sessionId = 'principal-b-session'
        accessToken = 'principal-b-token'
      }
      return config
    })
    request.instance.defaults.adapter = async (config) => {
      adapterCalls += 1
      return Promise.reject({
        config,
        response: { status: 401, data: { message: 'old session expired' } },
      })
    }

    await expect(request.post('/me/mutation', {}, { skipErrorToast: true })).rejects.toMatchObject({
      name: 'BrowserSessionChangedError',
    })
    expect(adapterCalls).toBe(1)
    expect(accessToken).toBe('principal-b-token')
  })
})
