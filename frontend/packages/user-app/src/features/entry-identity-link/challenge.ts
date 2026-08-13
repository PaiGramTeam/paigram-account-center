const storageKey = 'entry-identity-link:challenge'
const challengePattern = /^[A-Za-z0-9_-]{43}$/
const terminalChallengeCodes = new Set([
  'entry_identity_link_not_found',
  'entry_identity_link_expired',
  'entry_identity_link_consumed',
  'entry_identity_link_conflict',
  'entry_identity_namespace_unavailable',
])

interface FragmentLocation {
  hash: string
  pathname: string
  search: string
}

interface FragmentHistory {
  state: unknown
  replaceState(data: unknown, unused: string, url?: string | URL | null): void
}

interface ChallengeStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export function captureEntryIdentityChallenge(
  location: FragmentLocation = window.location,
  history: FragmentHistory = window.history,
  storage: ChallengeStorage = window.sessionStorage
): string | null {
  if (location.hash) {
    const fragment = new URLSearchParams(location.hash.slice(1))
    const candidate = fragment.get('challenge')
    history.replaceState(history.state, '', `${location.pathname}${location.search}`)
    if (candidate && challengePattern.test(candidate)) {
      storage.setItem(storageKey, candidate)
    }
  }
  const stored = storage.getItem(storageKey)
  return stored && challengePattern.test(stored) ? stored : null
}

export function clearEntryIdentityChallenge(storage: ChallengeStorage = window.sessionStorage): void {
  storage.removeItem(storageKey)
}

export function isTerminalEntryIdentityChallengeError(error: unknown): boolean {
  const code = (error as { code?: unknown })?.code
  return typeof code === 'string' && terminalChallengeCodes.has(code)
}
