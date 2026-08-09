import { afterEach, describe, expect, test } from 'bun:test'
import { resolveWorkers } from '../../playwright.workers'

const variable = 'PLAYWRIGHT_TEST_WORKERS'

afterEach(() => {
  delete process.env[variable]
})

describe('resolveWorkers', () => {
  test('returns the fallback when the variable is not configured', () => {
    expect(resolveWorkers(variable, '50%')).toBe('50%')
  })

  test('accepts positive integers and CPU percentages', () => {
    process.env[variable] = '4'
    expect(resolveWorkers(variable, '50%')).toBe(4)

    process.env[variable] = '75%'
    expect(resolveWorkers(variable, '50%')).toBe('75%')
  })

  test('rejects invalid values', () => {
    process.env[variable] = '0'
    expect(() => resolveWorkers(variable, '50%')).toThrow(`${variable} must be a positive integer or CPU percentage`)
  })
})
