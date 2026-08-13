interface EntryIdentityUnlinkError {
  code?: unknown
  status?: unknown
}

export const isMissingEntryIdentityUnlinkOperation = (error: unknown): boolean =>
  (error as EntryIdentityUnlinkError | null)?.code === 'entry_identity_unlink_operation_not_found'

export const isRetryableEntryIdentityUnlinkError = (error: unknown): boolean => {
  const status = (error as EntryIdentityUnlinkError | null)?.status
  if (typeof status !== 'number') return true
  return status >= 500
}
