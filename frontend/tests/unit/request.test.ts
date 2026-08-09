import { describe, expect, test } from 'bun:test'
import { normalizeApiError } from '../../packages/shared-components/src/api/request'

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
