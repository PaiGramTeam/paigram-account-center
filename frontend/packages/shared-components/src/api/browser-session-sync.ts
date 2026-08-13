import { BrowserSessionBroker, browserSessionBroker, type BrowserSessionEvent } from './session-broker'

export interface BrowserSessionSyncState {
  accessToken: string
  authenticated: boolean
  profileUserId: number | string | null
}

export interface BrowserSessionSyncAdapter {
  readState(): BrowserSessionSyncState
  clearSession(): void
  setAccessToken(accessToken: string): void
  loadProfile(userId: number): Promise<void>
  navigateBeforePrincipalChange(): Promise<void>
  navigateToLogin(): Promise<void>
  navigateAfterAuthentication(principalChanged: boolean): void | Promise<void>
}

export function synchronizeBrowserSession(
  adapter: BrowserSessionSyncAdapter,
  broker: BrowserSessionBroker = browserSessionBroker
): () => void {
  return broker.subscribe((event) => {
    void applyBrowserSessionEvent(event, adapter, broker)
  })
}

async function applyBrowserSessionEvent(
  event: BrowserSessionEvent,
  adapter: BrowserSessionSyncAdapter,
  broker: BrowserSessionBroker
): Promise<void> {
  if (event.type === 'ended') {
    if (!adapter.readState().authenticated) return
    adapter.clearSession()
    await adapter.navigateToLogin()
    return
  }

  const session = event.session
  const initialState = adapter.readState()
  const principalChanged =
    initialState.profileUserId !== null && String(initialState.profileUserId) !== String(session.userId)
  if (principalChanged) {
    adapter.clearSession()
    await adapter.navigateBeforePrincipalChange()
    if (!broker.isSessionCurrent(session)) return
  }
  if (!broker.isSessionCurrent(session)) return

  adapter.setAccessToken(session.accessToken)
  try {
    const state = adapter.readState()
    if (state.profileUserId === null || String(state.profileUserId) !== String(session.userId)) {
      await adapter.loadProfile(session.userId)
    }
  } catch {
    if (broker.isSessionCurrent(session)) {
      adapter.clearSession()
      await adapter.navigateToLogin()
    }
    return
  }
  if (!broker.isSessionCurrent(session)) {
    if (adapter.readState().accessToken === session.accessToken) adapter.clearSession()
    return
  }
  await adapter.navigateAfterAuthentication(principalChanged)
}
