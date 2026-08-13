import { describe, expect, test } from 'bun:test'
import {
  isMissingEntryIdentityUnlinkOperation,
  isRetryableEntryIdentityUnlinkError,
} from '../../packages/user-app/src/features/entry-identity-link/unlink'

describe('entry identity unlink failures', () => {
  test('recognizes the stable missing-operation code', () => {
    expect(isMissingEntryIdentityUnlinkOperation({ code: 'entry_identity_unlink_operation_not_found' })).toBe(true)
    expect(isMissingEntryIdentityUnlinkOperation({ message: 'not_found' })).toBe(false)
  })

  test('only retries transport and server failures', () => {
    expect(isRetryableEntryIdentityUnlinkError(new Error('network unavailable'))).toBe(true)
    expect(isRetryableEntryIdentityUnlinkError({ status: 503 })).toBe(true)
    expect(isRetryableEntryIdentityUnlinkError({ status: 404 })).toBe(false)
    expect(isRetryableEntryIdentityUnlinkError({ status: 401 })).toBe(false)
  })
})
