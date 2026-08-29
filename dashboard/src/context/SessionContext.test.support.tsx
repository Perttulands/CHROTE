import { expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { createElement } from 'react'
import { SessionProvider, useSession } from './SessionContext'
import { ToastProvider, useToast } from './ToastContext'

export function setViewportWidth(width: number) {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    writable: true,
    value: width,
  })
}

// Keep the default refresh pending. Tests that exercise refreshSessions install
// an explicit response, so unrelated tests do not receive an async provider
// update after their assertions have completed.
const defaultFetch = vi.fn((input: RequestInfo | URL) => {
  if (String(input) === '/api/tmux/sessions') return new Promise<never>(() => {})
  return Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ sessions: [], grouped: {}, timestamp: new Date().toISOString() }),
    text: () => Promise.resolve(''),
  })
})
vi.stubGlobal('fetch', defaultFetch)

// Mock localStorage
export const store: Record<string, string> = {}
vi.stubGlobal('localStorage', {
  getItem: (key: string) => store[key] ?? null,
  setItem: (key: string, val: string) => { store[key] = val },
  removeItem: (key: string) => { delete store[key] },
  clear: () => { Object.keys(store).forEach(k => delete store[k]) },
  length: 0,
  key: () => null,
})

beforeEach(() => {
  vi.stubGlobal('fetch', defaultFetch)
  defaultFetch.mockClear()
})

export function Wrapper({ children }: { children: React.ReactNode }) {
  return createElement(ToastProvider, null,
    createElement(SessionProvider, null, children))
}

export function renderSession() {
  return renderHook(() => useSession(), { wrapper: Wrapper })
}

export function renderSessionWithToast() {
  return renderHook(() => ({ session: useSession(), toast: useToast() }), { wrapper: Wrapper })
}

export function storedDashboardState() {
  const stored = store['chrote-dashboard-state']
  expect(stored).toBeDefined()
  return JSON.parse(stored)
}

export function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

export async function flushPromises() {
  for (let index = 0; index < 12; index += 1) await Promise.resolve()
}

export function sessionResponse(data: Record<string, unknown>, ok = true) {
  return {
    ok,
    json: vi.fn(() => Promise.resolve(data)),
    text: vi.fn(() => Promise.resolve(data.error ? JSON.stringify(data) : '')),
  }
}

export function stubDeferredSessionFetch() {
  const requests: Array<{
    response: ReturnType<typeof deferred<any>>
    signal: AbortSignal | null
  }> = []
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    if (String(input) === '/api/tmux/sessions' && !init?.method) {
      const response = deferred<any>()
      requests.push({ response, signal: init?.signal as AbortSignal | null })
      return response.promise
    }
    return Promise.resolve(sessionResponse({}))
  })
  vi.stubGlobal('fetch', fetchMock)
  return { fetchMock, requests }
}
