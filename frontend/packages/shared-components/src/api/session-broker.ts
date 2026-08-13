export interface BrowserSessionSnapshot {
  accessToken: string
  userId: number
}

export interface BrowserAuthenticationResult<T> {
  value: T
  session?: BrowserSessionSnapshot
}

export interface BrowserSessionRefreshOptions {
  rejectedAccessToken?: string
  onFailure?: (error: unknown) => void | Promise<void>
  commit?: (session: BrowserSessionSnapshot) => void
}

export type BrowserSessionEvent =
  | { type: 'authenticated'; revision: string; sessionId: string; session: BrowserSessionSnapshot }
  | { type: 'ended'; revision: string }

interface SessionMarker {
  version: 1
  revision: string
  state: 'authenticated' | 'ended'
  sessionId?: string
}

type SessionMessage =
  | (BrowserSessionEvent & { version: 1; source: string })
  | { version: 1; source: string; type: 'request'; revision: string }

interface MessageEventLike {
  data: unknown
}

interface SessionChannel {
  postMessage(message: unknown): void
  addEventListener(type: 'message', listener: (event: MessageEventLike) => void): void
  removeEventListener(type: 'message', listener: (event: MessageEventLike) => void): void
}

interface SessionStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

interface SessionLockManager {
  request<T>(name: string, callback: () => Promise<T>): Promise<T>
}

interface StorageEventLike {
  key: string | null
  newValue: string | null
}

interface StorageEventTarget {
  addEventListener(type: 'storage', listener: (event: StorageEventLike) => void): void
  removeEventListener(type: 'storage', listener: (event: StorageEventLike) => void): void
}

export interface BrowserSessionBrokerOptions {
  source?: string
  channel?: SessionChannel
  storage?: SessionStorage
  storageEvents?: StorageEventTarget
  locks?: SessionLockManager
  requestTimeoutMs?: number
}

const markerKey = 'paigram.browser-session.v1'
const lockName = 'paigram.browser-session.refresh.v1'
const revisionPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

export class BrowserSessionEndedError extends Error {
  constructor() {
    super('Browser session has ended')
    this.name = 'BrowserSessionEndedError'
  }
}

export class BrowserSessionCapabilityError extends Error {
  constructor() {
    super('Secure cross-tab browser session coordination is unavailable')
    this.name = 'BrowserSessionCapabilityError'
  }
}

export function isTerminalBrowserSessionFailure(error: unknown): boolean {
  const status = (error as { status?: unknown } | null)?.status
  return status === 401 || status === 403
}

export class BrowserSessionBroker {
  private readonly source: string
  private readonly channel?: SessionChannel
  private readonly storage?: SessionStorage
  private readonly storageEvents?: StorageEventTarget
  private readonly locks?: SessionLockManager
  private readonly requestTimeoutMs: number
  private readonly subscribers = new Set<(event: BrowserSessionEvent) => void>()
  private current: { revision: string; sessionId: string; session: BrowserSessionSnapshot } | null = null
  private lastNotifiedRevision = ''

  constructor(options: BrowserSessionBrokerOptions = {}) {
    this.source = options.source ?? createRevision()
    this.channel = options.channel
    this.storage = options.storage
    this.storageEvents = options.storageEvents
    this.locks = options.locks
    this.requestTimeoutMs = options.requestTimeoutMs ?? 250
    this.channel?.addEventListener('message', this.onMessage)
    this.storageEvents?.addEventListener('storage', this.onStorage)
  }

  subscribe(listener: (event: BrowserSessionEvent) => void): () => void {
    let lastDeliveredRevision = ''
    const subscriber = (event: BrowserSessionEvent) => {
      if (lastDeliveredRevision === event.revision) return
      lastDeliveredRevision = event.revision
      listener(event)
    }
    this.subscribers.add(subscriber)
    queueMicrotask(() => {
      if (!this.subscribers.has(subscriber)) return
      const event = this.currentEvent()
      if (event) subscriber(event)
    })
    return () => this.subscribers.delete(subscriber)
  }

  isEnded(): boolean {
    return this.readMarker()?.state === 'ended'
  }

  isCurrent(revision: string): boolean {
    return this.readMarker()?.revision === revision
  }

  currentSessionId(): string {
    const marker = this.readMarker()
    if (
      marker?.state !== 'authenticated' ||
      !marker.sessionId ||
      marker.revision !== this.current?.revision ||
      marker.sessionId !== this.current.sessionId
    ) {
      return ''
    }
    return marker.sessionId
  }

  isSessionCurrent(session: BrowserSessionSnapshot): boolean {
    const marker = this.readMarker()
    return (
      marker?.state === 'authenticated' &&
      marker.sessionId === this.current?.sessionId &&
      marker.revision === this.current?.revision &&
      this.current.session.accessToken === session.accessToken &&
      this.current.session.userId === session.userId
    )
  }

  isUserSessionCurrent(userId: number): boolean {
    const marker = this.readMarker()
    return (
      Number.isSafeInteger(userId) &&
      userId > 0 &&
      marker?.state === 'authenticated' &&
      marker.sessionId === this.current?.sessionId &&
      marker.revision === this.current?.revision &&
      this.current.session.userId === userId
    )
  }

  establish(session: BrowserSessionSnapshot): void {
    this.assertCoordinationCapabilities()
    this.publishAuthenticated(session, createRevision())
  }

  private publishAuthenticated(session: BrowserSessionSnapshot, sessionId: string): void {
    if (!validSession(session)) throw new Error('Invalid browser session snapshot')
    const revision = createRevision()
    const marker: SessionMarker = { version: 1, revision, state: 'authenticated', sessionId }
    this.current = { revision, sessionId, session }
    this.writeMarker(marker)
    this.channel?.postMessage({
      version: 1,
      source: this.source,
      type: 'authenticated',
      revision,
      sessionId,
      session,
    })
  }

  invalidate(): void {
    const revision = createRevision()
    this.current = null
    this.writeMarker({ version: 1, revision, state: 'ended' })
    this.channel?.postMessage({ version: 1, source: this.source, type: 'ended', revision })
  }

  invalidateIfCurrent(accessToken: string, afterInvalidate?: () => void | Promise<void>): Promise<boolean> {
    return this.exclusive(async () => {
      const marker = this.readMarker()
      if (
        !accessToken ||
        marker?.state !== 'authenticated' ||
        marker.revision !== this.current?.revision ||
        this.current.session.accessToken !== accessToken
      ) {
        return false
      }
      this.invalidate()
      await afterInvalidate?.()
      return true
    })
  }

  async refresh(
    start: () => Promise<BrowserSessionSnapshot>,
    options: BrowserSessionRefreshOptions = {}
  ): Promise<BrowserSessionSnapshot> {
    this.assertCoordinationCapabilities()
    const baseline = this.readMarker()
    if (baseline?.state === 'ended') throw new BrowserSessionEndedError()
    return this.exclusive(async () => {
      const marker = this.readMarker()
      if (marker?.state === 'ended') throw new BrowserSessionEndedError()
      if (marker?.state === 'authenticated') {
        const cached = await this.snapshotForRevision(marker.revision)
        if (cached && cached.accessToken !== options.rejectedAccessToken) {
          options.commit?.(cached)
          return cached
        }
      }
      const sessionId = marker?.state === 'authenticated' && marker.sessionId ? marker.sessionId : createRevision()
      let session: BrowserSessionSnapshot
      try {
        session = await start()
      } catch (error) {
        await options.onFailure?.(error)
        throw error
      }
      options.commit?.(session)
      this.publishAuthenticated(session, sessionId)
      return session
    })
  }

  authenticate<T>(start: () => Promise<BrowserAuthenticationResult<T>>): Promise<T> {
    return this.exclusive(async () => {
      const result = await start()
      if (result.session) this.establish(result.session)
      return result.value
    })
  }

  async logout(end: () => Promise<void>): Promise<void> {
    if (!this.hasCoordinationCapabilities()) {
      try {
        await end()
      } finally {
        this.invalidate()
      }
      return
    }
    await this.exclusive(async () => {
      try {
        await end()
      } finally {
        this.invalidate()
      }
    })
  }

  logoutIfCurrent(session: BrowserSessionSnapshot, end: () => Promise<void>): Promise<boolean> {
    return this.exclusive(async () => {
      if (!this.isSessionCurrent(session)) return false
      try {
        await end()
      } finally {
        this.invalidate()
      }
      return true
    })
  }

  dispose(): void {
    this.channel?.removeEventListener('message', this.onMessage)
    this.storageEvents?.removeEventListener('storage', this.onStorage)
    this.subscribers.clear()
  }

  private async exclusive<T>(callback: () => Promise<T>): Promise<T> {
    this.assertCoordinationCapabilities()
    return this.locks!.request(lockName, callback)
  }

  private assertCoordinationCapabilities(): void {
    if (!this.hasCoordinationCapabilities()) throw new BrowserSessionCapabilityError()
  }

  private hasCoordinationCapabilities(): boolean {
    return Boolean(this.locks && this.channel && this.storage)
  }

  private async snapshotForRevision(revision: string): Promise<BrowserSessionSnapshot | null> {
    if (this.current?.revision === revision) return this.current.session
    if (!this.channel) return null
    return new Promise((resolve) => {
      let settled = false
      const finish = (session: BrowserSessionSnapshot | null) => {
        if (settled) return
        settled = true
        windowClearTimeout(timer)
        unsubscribe()
        resolve(session)
      }
      const unsubscribe = this.subscribe((event) => {
        if (event.type === 'authenticated' && event.revision === revision) finish(event.session)
        if (event.type === 'ended') finish(null)
      })
      const timer = windowSetTimeout(() => finish(null), this.requestTimeoutMs)
      this.channel?.postMessage({ version: 1, source: this.source, type: 'request', revision })
      if (this.current?.revision === revision) finish(this.current.session)
    })
  }

  private readonly onMessage = (event: MessageEventLike): void => {
    const message = parseMessage(event.data)
    if (!message || message.source === this.source) return
    if (message.type === 'request') {
      if (this.current?.revision === message.revision) {
        this.channel?.postMessage({
          version: 1,
          source: this.source,
          type: 'authenticated',
          revision: message.revision,
          sessionId: this.current.sessionId,
          session: this.current.session,
        })
      }
      return
    }
    const marker = this.readMarker()
    if (!marker || marker.revision !== message.revision || marker.state !== message.type) return
    if (message.type === 'authenticated') {
      if (marker.sessionId !== message.sessionId) return
      this.current = { revision: message.revision, sessionId: message.sessionId, session: message.session }
    } else this.current = null
    this.notify(message)
  }

  private readonly onStorage = (event: StorageEventLike): void => {
    if (event.key !== markerKey) return
    const marker = parseMarker(event.newValue)
    if (!marker) return
    if (marker.state === 'ended') {
      this.current = null
      this.notify({ type: 'ended', revision: marker.revision })
      return
    }
    if (this.current?.revision !== marker.revision) {
      void this.snapshotForRevision(marker.revision)
    }
  }

  private notify(event: BrowserSessionEvent): void {
    if (this.lastNotifiedRevision === event.revision) return
    this.lastNotifiedRevision = event.revision
    for (const subscriber of this.subscribers) subscriber(event)
  }

  private currentEvent(): BrowserSessionEvent | null {
    const marker = this.readMarker()
    if (!marker) return null
    if (marker.state === 'ended') return { type: 'ended', revision: marker.revision }
    if (marker.revision !== this.current?.revision) return null
    return {
      type: 'authenticated',
      revision: marker.revision,
      sessionId: this.current.sessionId,
      session: this.current.session,
    }
  }

  private readMarker(): SessionMarker | null {
    return parseMarker(this.storage?.getItem(markerKey) ?? null)
  }

  private writeMarker(marker: SessionMarker): void {
    this.storage?.setItem(markerKey, JSON.stringify(marker))
  }
}

function parseMarker(raw: string | null): SessionMarker | null {
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as Partial<SessionMarker>
    if (
      value.version !== 1 ||
      !validRevision(value.revision) ||
      (value.state !== 'authenticated' && value.state !== 'ended')
    ) {
      return null
    }
    if (value.state === 'authenticated' && !validRevision(value.sessionId)) return null
    return value as SessionMarker
  } catch {
    return null
  }
}

function parseMessage(raw: unknown): SessionMessage | null {
  if (!raw || typeof raw !== 'object') return null
  const value = raw as Partial<SessionMessage>
  if (value.version !== 1 || !validRevision(value.source) || !validRevision(value.revision)) return null
  if (value.type === 'request' || value.type === 'ended') return value as SessionMessage
  if (value.type === 'authenticated' && validRevision(value.sessionId) && validSession(value.session)) {
    return value as SessionMessage
  }
  return null
}

function validSession(value: unknown): value is BrowserSessionSnapshot {
  if (!value || typeof value !== 'object') return false
  const session = value as Partial<BrowserSessionSnapshot>
  return (
    typeof session.accessToken === 'string' &&
    session.accessToken.length > 0 &&
    session.accessToken.length <= 4096 &&
    Number.isSafeInteger(session.userId) &&
    Number(session.userId) > 0
  )
}

function validRevision(value: unknown): value is string {
  return typeof value === 'string' && revisionPattern.test(value)
}

function createRevision(): string {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID()
  const bytes = new Uint8Array(16)
  if (globalThis.crypto?.getRandomValues) globalThis.crypto.getRandomValues(bytes)
  else for (let index = 0; index < bytes.length; index += 1) bytes[index] = Math.floor(Math.random() * 256)
  bytes[6] = (bytes[6]! & 0x0f) | 0x40
  bytes[8] = (bytes[8]! & 0x3f) | 0x80
  const hex = [...bytes].map((value) => value.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

function windowSetTimeout(callback: () => void, milliseconds: number): ReturnType<typeof setTimeout> {
  return globalThis.setTimeout(callback, milliseconds)
}

function windowClearTimeout(timer: ReturnType<typeof setTimeout>): void {
  globalThis.clearTimeout(timer)
}

function createBrowserBroker(): BrowserSessionBroker {
  const channel =
    typeof BroadcastChannel === 'undefined' ? undefined : new BroadcastChannel('paigram.browser-session.v1')
  const storage = typeof localStorage === 'undefined' ? undefined : localStorage
  const storageEvents = typeof window === 'undefined' ? undefined : window
  const locks = typeof navigator === 'undefined' ? undefined : navigator.locks
  return new BrowserSessionBroker({ channel, storage, storageEvents, locks })
}

export const browserSessionBroker = createBrowserBroker()
