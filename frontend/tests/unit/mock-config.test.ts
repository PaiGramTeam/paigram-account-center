import { describe, expect, test } from 'bun:test'
import { resolveMockOptions } from '../../packages/shared-components/src/mocks/config'

describe('resolveMockOptions', () => {
  test('uses the default scenario for missing or invalid values', () => {
    expect(resolveMockOptions()).toEqual({ scenario: 'default' })
    expect(resolveMockOptions({ environment: { scenario: 'unknown' } })).toEqual({ scenario: 'default' })
  })

  test('gives the query string precedence over the environment', () => {
    expect(
      resolveMockOptions({
        environment: { scenario: 'slow' },
        search: '?mockScenario=error',
      })
    ).toEqual({ scenario: 'error' })
  })
})
