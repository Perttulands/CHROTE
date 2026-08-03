import { describe, expect, it, vi } from 'vitest'
import {
  CHUNK_RELOAD_COOLDOWN_MS,
  CHUNK_RELOAD_STORAGE_KEY,
  installChunkReloadRecovery,
} from './chunkReloadRecovery'

function makeStorage(initial: Record<string, string> = {}) {
  const map = new Map(Object.entries(initial))
  return {
    getItem: (key: string) => map.get(key) ?? null,
    setItem: (key: string, value: string) => {
      map.set(key, value)
    },
    map,
  }
}

function firePreloadError(target: EventTarget): Event {
  const event = new Event('vite:preloadError', { cancelable: true })
  target.dispatchEvent(event)
  return event
}

describe('installChunkReloadRecovery', () => {
  it('reloads once on first chunk failure, cancels the event, and records the attempt', () => {
    const target = new EventTarget()
    const storage = makeStorage()
    const reload = vi.fn()
    installChunkReloadRecovery({ target, storage, reload, now: () => 1000 })

    const event = firePreloadError(target)

    expect(reload).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)
    expect(storage.map.get(CHUNK_RELOAD_STORAGE_KEY)).toBe('1000')
  })

  it('swallows follow-on failures while the reload is in flight without reloading again', () => {
    const target = new EventTarget()
    const reload = vi.fn()
    installChunkReloadRecovery({ target, storage: makeStorage(), reload, now: () => 1000 })

    firePreloadError(target)
    const second = firePreloadError(target)

    expect(reload).toHaveBeenCalledTimes(1)
    expect(second.defaultPrevented).toBe(true)
  })

  it('propagates the error instead of reloading when a recovery reload already happened within the cooldown', () => {
    const target = new EventTarget()
    const reload = vi.fn()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    installChunkReloadRecovery({
      target,
      storage: makeStorage({ [CHUNK_RELOAD_STORAGE_KEY]: '1000' }),
      reload,
      now: () => 1000 + CHUNK_RELOAD_COOLDOWN_MS - 1,
    })

    const event = firePreloadError(target)

    expect(reload).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
    expect(consoleError).toHaveBeenCalled()
    consoleError.mockRestore()
  })

  it('re-arms after the cooldown expires', () => {
    const target = new EventTarget()
    const reload = vi.fn()
    installChunkReloadRecovery({
      target,
      storage: makeStorage({ [CHUNK_RELOAD_STORAGE_KEY]: '1000' }),
      reload,
      now: () => 1000 + CHUNK_RELOAD_COOLDOWN_MS,
    })

    const event = firePreloadError(target)

    expect(reload).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)
  })

  it('still reloads once per page load when storage is unavailable', () => {
    const target = new EventTarget()
    const reload = vi.fn()
    const throwingStorage = {
      getItem: () => {
        throw new Error('storage disabled')
      },
      setItem: () => {
        throw new Error('storage disabled')
      },
    }
    installChunkReloadRecovery({ target, storage: throwingStorage, reload, now: () => 1000 })

    firePreloadError(target)
    firePreloadError(target)

    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('stops listening after uninstall', () => {
    const target = new EventTarget()
    const reload = vi.fn()
    const uninstall = installChunkReloadRecovery({ target, storage: makeStorage(), reload, now: () => 1000 })

    uninstall()
    const event = firePreloadError(target)

    expect(reload).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })
})
