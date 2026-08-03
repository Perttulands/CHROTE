// Recovers open tabs from post-deploy chunk-load failures. Every non-terminal
// view is a lazy route-level chunk; deploying a new binary replaces the
// embedded content-hashed assets, so a tab opened before the deploy 404s on
// its next dynamic import. Vite dispatches a cancelable `vite:preloadError`
// on window when a dynamic import or one of its preloaded deps fails —
// cancelling it suppresses the rethrow. First failure: reload the tab to pick
// up the new index.html. Failure again within the cooldown means the reload
// did not fix it (server broken, not a stale bundle) — let the error
// propagate to the view's ErrorBoundary instead of looping.

interface VitePreloadErrorEvent extends Event {
  payload?: unknown
}

export const CHUNK_RELOAD_STORAGE_KEY = 'chrote-chunk-reload-at'
export const CHUNK_RELOAD_COOLDOWN_MS = 60_000

export interface ChunkReloadRecoveryDeps {
  target?: Pick<EventTarget, 'addEventListener' | 'removeEventListener'>
  storage?: Pick<Storage, 'getItem' | 'setItem'>
  reload?: () => void
  now?: () => number
  cooldownMs?: number
}

export function installChunkReloadRecovery(deps: ChunkReloadRecoveryDeps = {}): () => void {
  const target = deps.target ?? window
  const reload = deps.reload ?? (() => window.location.reload())
  const now = deps.now ?? (() => Date.now())
  const cooldownMs = deps.cooldownMs ?? CHUNK_RELOAD_COOLDOWN_MS

  // sessionStorage is per-tab and survives the reload, which is what makes the
  // loop guard work. If it is unavailable the in-memory flag still bounds this
  // page load to one reload; a fresh page cannot remember prior attempts.
  const readLastReloadAt = (): number => {
    try {
      const storage = deps.storage ?? window.sessionStorage
      return Number(storage.getItem(CHUNK_RELOAD_STORAGE_KEY)) || 0
    } catch {
      return 0
    }
  }
  const writeLastReloadAt = (timestamp: number): void => {
    try {
      const storage = deps.storage ?? window.sessionStorage
      storage.setItem(CHUNK_RELOAD_STORAGE_KEY, String(timestamp))
    } catch {
      // In-memory `reloading` flag remains the only guard.
    }
  }

  let reloading = false

  const onPreloadError = (event: Event): void => {
    if (reloading) {
      // A reload is already on its way; swallow follow-on failures from other
      // deps of the same import so they do not flash the error boundary.
      event.preventDefault()
      return
    }
    const timestamp = now()
    const lastReloadAt = readLastReloadAt()
    if (lastReloadAt && timestamp - lastReloadAt < cooldownMs) {
      const payload = (event as VitePreloadErrorEvent).payload
      console.error('Chunk load failed again after a recovery reload; giving up.', payload)
      return
    }
    reloading = true
    writeLastReloadAt(timestamp)
    event.preventDefault()
    reload()
  }

  target.addEventListener('vite:preloadError', onPreloadError)
  return () => target.removeEventListener('vite:preloadError', onPreloadError)
}
