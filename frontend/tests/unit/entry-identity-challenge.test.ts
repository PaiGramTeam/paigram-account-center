import { describe, expect, test } from 'bun:test'
import {
  captureEntryIdentityChallenge,
  clearEntryIdentityChallenge,
  isTerminalEntryIdentityChallengeError,
} from '../../packages/user-app/src/features/entry-identity-link/challenge'

function memoryStorage() {
  const values = new Map<string, string>()
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    removeItem: (key: string) => values.delete(key),
  }
}

describe('entry identity challenge fragment', () => {
  test('stores a valid fragment challenge and immediately scrubs it from history', () => {
    const challenge = 'A'.repeat(43)
    const storage = memoryStorage()
    const replacements: string[] = []
    const captured = captureEntryIdentityChallenge(
      { hash: `#challenge=${challenge}`, pathname: '/entry-identity-link', search: '' },
      { state: null, replaceState: (_data, _unused, url) => replacements.push(String(url)) },
      storage
    )

    expect(captured).toBe(challenge)
    expect(replacements).toEqual(['/entry-identity-link'])
    expect(replacements[0]).not.toContain(challenge)
  })

  test('survives login navigation in session storage and clears after terminal action', () => {
    const challenge = 'B'.repeat(43)
    const storage = memoryStorage()
    captureEntryIdentityChallenge(
      { hash: `#challenge=${challenge}`, pathname: '/entry-identity-link', search: '' },
      { state: null, replaceState: () => undefined },
      storage
    )

    expect(
      captureEntryIdentityChallenge(
        { hash: '', pathname: '/entry-identity-link', search: '' },
        { state: null, replaceState: () => undefined },
        storage
      )
    ).toBe(challenge)
    clearEntryIdentityChallenge(storage)
    expect(storage.getItem('entry-identity-link:challenge')).toBeNull()
  })

  test('scrubs but rejects malformed fragment values', () => {
    const storage = memoryStorage()
    const replacements: string[] = []
    const captured = captureEntryIdentityChallenge(
      { hash: '#challenge=not-a-token', pathname: '/entry-identity-link', search: '?source=bot' },
      { state: null, replaceState: (_data, _unused, url) => replacements.push(String(url)) },
      storage
    )

    expect(captured).toBeNull()
    expect(replacements).toEqual(['/entry-identity-link?source=bot'])
  })

  test('recognizes terminal server codes without relying on translated messages', () => {
    expect(isTerminalEntryIdentityChallengeError({ code: 'entry_identity_link_expired', message: '任意文案' })).toBe(
      true
    )
    expect(isTerminalEntryIdentityChallengeError({ code: 'entry_identity_unlink_pending' })).toBe(false)
    expect(isTerminalEntryIdentityChallengeError(new Error('entry identity link expired'))).toBe(false)
  })
})
