import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { createElement, StrictMode } from 'react'
import { SessionProvider, useSession } from './SessionContext'
import { ToastProvider, useToast } from './ToastContext'
import { featureFlagKey } from '../featureFlags'
import { DEFAULT_SETTINGS, DEFAULT_TMUX_APPEARANCE, resolveLaunchUser } from '../types'
import type { SendToSessionOutcome } from '../types'

function setViewportWidth(width: number) {
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
const store: Record<string, string> = {}
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

function Wrapper({ children }: { children: React.ReactNode }) {
  return createElement(ToastProvider, null,
    createElement(SessionProvider, null, children))
}

function renderSession() {
  return renderHook(() => useSession(), { wrapper: Wrapper })
}

function renderSessionWithToast() {
  return renderHook(() => ({ session: useSession(), toast: useToast() }), { wrapper: Wrapper })
}

function storedDashboardState() {
  const stored = store['chrote-dashboard-state']
  expect(stored).toBeDefined()
  return JSON.parse(stored)
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

async function flushPromises() {
  for (let index = 0; index < 12; index += 1) await Promise.resolve()
}

function sessionResponse(data: Record<string, unknown>, ok = true) {
  return {
    ok,
    json: vi.fn(() => Promise.resolve(data)),
    text: vi.fn(() => Promise.resolve(data.error ? JSON.stringify(data) : '')),
  }
}

function stubDeferredSessionFetch() {
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

// ──────────────────────────────────────────────
// 0. localStorage key and persisted shape contract
// ──────────────────────────────────────────────
describe('dashboard persisted storage contract', () => {
  beforeEach(() => {
    localStorage.clear()
    setViewportWidth(1280)
  })

  it('pins the exact dashboard state, presets, and feature flag key strings', () => {
    const { result } = renderSession()

    act(() => {
      result.current.saveCurrentLayout('baseline')
    })

    expect(store).toHaveProperty('chrote-dashboard-state')
    expect(store).toHaveProperty('chrote-dashboard-presets')
    expect(featureFlagKey('uiV2')).toBe('chrote-ui-v2')
    expect(featureFlagKey('filesPersistTabState')).toBe('chrote-files-persist-tab-state')
    expect(featureFlagKey('serverStatusTab')).toBe('chrote-server-status-tab')
    expect(DEFAULT_SETTINGS.mouseScroll).toBe(true)

    const presets = JSON.parse(store['chrote-dashboard-presets'])
    expect(presets).toHaveLength(1)
    expect(presets[0].name).toBe('baseline')
  })

  it('loads V3 viewport layouts as a normalized stored-state fixture', () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [
                { id: 'legacy-id', boundSessions: ['desk-a'], activeSession: 'desk-a', colorIndex: 3 },
                { id: 'also-legacy', boundSessions: ['desk-b'], activeSession: null },
              ],
              windowCount: 2,
            },
            terminal2: {
              windows: [
                { id: 'wrong-terminal', boundSessions: ['desk-c'], activeSession: 'desk-c', colorIndex: 7 },
              ],
              windowCount: 1,
            },
          },
        },
        mobile: {
          workspaces: {
            terminal1: {
              windows: [
                { id: 'mobile-legacy', boundSessions: ['mobile-a'], activeSession: 'mobile-a', colorIndex: 1 },
              ],
              windowCount: 1,
            },
            terminal2: {
              windows: [],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: true,
      settings: {
        fontSize: 18,
        tmuxAppearance: {
          statusFg: '#ffffff',
        },
      },
    }))

    const { result } = renderSession()

    expect(result.current.workspaces).toEqual({
      terminal1: {
        windows: [
          { id: 'terminal1-window-0', boundSessions: ['desk-a'], activeSession: 'desk-a', colorIndex: 3 },
          { id: 'terminal1-window-1', boundSessions: ['desk-b'], activeSession: null, colorIndex: 1 },
          { id: 'terminal1-window-2', boundSessions: [], activeSession: null, colorIndex: 2 },
          { id: 'terminal1-window-3', boundSessions: [], activeSession: null, colorIndex: 3 },
        ],
        windowCount: 2,
      },
      terminal2: {
        windows: [
          { id: 'terminal2-window-0', boundSessions: ['desk-c'], activeSession: 'desk-c', colorIndex: 7 },
          { id: 'terminal2-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
          { id: 'terminal2-window-2', boundSessions: [], activeSession: null, colorIndex: 2 },
          { id: 'terminal2-window-3', boundSessions: [], activeSession: null, colorIndex: 3 },
        ],
        windowCount: 1,
      },
      terminal3: {
        windows: [
          { id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
          { id: 'terminal3-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
          { id: 'terminal3-window-2', boundSessions: [], activeSession: null, colorIndex: 2 },
          { id: 'terminal3-window-3', boundSessions: [], activeSession: null, colorIndex: 3 },
        ],
        windowCount: 2,
      },
    })
    expect(result.current.sidebarCollapsed).toBe(true)
    expect(result.current.settings).toEqual({
      ...DEFAULT_SETTINGS,
      fontSize: 18,
      tmuxAppearance: {
        ...DEFAULT_TMUX_APPEARANCE,
        statusFg: '#ffffff',
      },
    })
  })

  it('merges older saved settings with current defaults', () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      workspaces: {
        terminal1: {
          windows: [
            { id: 'terminal1-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
          ],
          windowCount: 1,
        },
        terminal2: {
          windows: [
            { id: 'terminal2-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
          ],
          windowCount: 1,
        },
      },
      sidebarCollapsed: false,
      settingsSchemaVersion: 2,
      settings: {
        fontSize: 17,
        tmuxAppearance: {
          paneBorderActive: '#abcdef',
        },
      },
    }))

    const { result } = renderSession()

    expect(result.current.settings).toEqual({
      ...DEFAULT_SETTINGS,
      fontSize: 17,
      tmuxAppearance: {
        ...DEFAULT_TMUX_APPEARANCE,
        paneBorderActive: '#abcdef',
      },
    })
  })

  it('applies saved tmux mouse mode on initial load and when settings change', async () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {},
      settings: { ...DEFAULT_SETTINGS, mouseScroll: false },
    }))
    const fetchMock = vi.mocked(fetch as any)
    fetchMock.mockClear()

    const { result } = renderSession()
    const hasMouseCall = (enabled: boolean) => fetchMock.mock.calls.some((call: unknown[]) => {
      const [url, init] = call as [RequestInfo | URL, RequestInit | undefined]
      return String(url) === '/api/tmux/mouse' &&
        init?.method === 'POST' &&
        JSON.parse(String(init?.body)).enabled === enabled
    })

    await waitFor(() => expect(hasMouseCall(false)).toBe(true))

    fetchMock.mockClear()
    act(() => {
      result.current.updateSettings({ mouseScroll: true })
    })

    await waitFor(() => expect(hasMouseCall(true)).toBe(true))
  })

  it('preserves the legacy global session prefix without hardcoded user-key migration', () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      workspaces: {
        terminal1: {
          windows: [
            { id: 'terminal1-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
          ],
          windowCount: 1,
        },
        terminal2: {
          windows: [
            { id: 'terminal2-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
          ],
          windowCount: 1,
        },
      },
      sidebarCollapsed: false,
      settingsSchemaVersion: 2,
      settings: {
        defaultSessionPrefix: 'legacy',
      },
    }))

    const { result } = renderSession()

    expect(result.current.settings.defaultSessionPrefix).toBe('legacy')
    expect(result.current.settings.terminalSessionPrefixes).toEqual({})
  })

  it('filters INIT-PENDING active sessions before persisting state', async () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [
                { id: 'terminal1-window-0', boundSessions: ['pending'], activeSession: 'INIT-PENDING', colorIndex: 0 },
              ],
              windowCount: 1,
            },
            terminal2: {
              windows: [
                { id: 'terminal2-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
              ],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: false,
      settings: DEFAULT_SETTINGS,
    }))

    const { result } = renderSession()

    await waitFor(() => {
      expect(result.current.workspaces.terminal1.windows[0].activeSession).toBeNull()
    })

    const persisted = storedDashboardState()
    expect(persisted.layoutsByViewport.desktop.workspaces.terminal1.windows[0]).toEqual({
      id: 'terminal1-window-0',
      boundSessions: ['pending'],
      activeSession: null,
      colorIndex: 0,
    })
  })
})

// ──────────────────────────────────────────────
// 1. migrateStoredState — V1 → V2 migration
// ──────────────────────────────────────────────
describe('migrateStoredState (via loadStoredState)', () => {
  beforeEach(() => {
    localStorage.clear()
    setViewportWidth(1280)
  })

  it('migrates V1 format (flat windows array) to V2 (workspaces object)', () => {
    // V1 stored format: windows array + windowCount at top level
    const v1State = {
      windows: [
        { id: 'window-0', boundSessions: ['sess-a', 'sess-b'], activeSession: 'sess-a', colorIndex: 0 },
        { id: 'window-1', boundSessions: ['sess-c'], activeSession: 'sess-c', colorIndex: 1 },
      ],
      windowCount: 2,
      sidebarCollapsed: true,
      settings: {
        terminalMode: 'tmux',
        fontSize: 16,
        theme: 'dark',
        autoRefreshInterval: 5000,
        defaultSessionPrefix: 'shell',
        musicVolume: 0.5,
        musicEnabled: false,
        tmuxAppearance: {
          statusBg: 'default',
          statusFg: '#6b9fff',
          paneBorderActive: '#6b9fff',
          paneBorderInactive: '#3a3a3a',
          modeStyleBg: '#6b9fff',
          modeStyleFg: '#0f0f0f',
        },
      },
    }

    localStorage.setItem('chrote-dashboard-state', JSON.stringify(v1State))

    const { result } = renderSession()

    // V1 windows migrate into terminal1
    const t1 = result.current.workspaces.terminal1
    expect(t1.windowCount).toBe(2)
    expect(t1.windows).toHaveLength(4)
    expect(t1.windows[0].boundSessions).toEqual(['sess-a', 'sess-b'])
    expect(t1.windows[0].activeSession).toBe('sess-a')
    expect(t1.windows[0].id).toBe('terminal1-window-0')
    expect(t1.windows[1].boundSessions).toEqual(['sess-c'])
    expect(t1.windows.map(window => window.id)).toEqual([
      'terminal1-window-0',
      'terminal1-window-1',
      'terminal1-window-2',
      'terminal1-window-3',
    ])

    // terminal2 gets fresh defaults
    const t2 = result.current.workspaces.terminal2
    expect(t2.windowCount).toBe(2)
    expect(t2.windows).toHaveLength(4)
    expect(t2.windows[0].boundSessions).toEqual([])
    expect(t2.windows[0].id).toBe('terminal2-window-0')

    // Sidebar state preserved
    expect(result.current.sidebarCollapsed).toBe(true)
  })

  it('loads V2 format directly without migration', () => {
    const v2State = {
      workspaces: {
        terminal1: {
          windows: [
            { id: 'terminal1-window-0', boundSessions: ['alpha'], activeSession: 'alpha', colorIndex: 0 },
          ],
          windowCount: 1,
        },
        terminal2: {
          windows: [
            { id: 'terminal2-window-0', boundSessions: ['beta'], activeSession: 'beta', colorIndex: 0 },
            { id: 'terminal2-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
          ],
          windowCount: 2,
        },
      },
      sidebarCollapsed: false,
      settings: {
        terminalMode: 'tmux',
        fontSize: 14,
        theme: 'matrix',
        autoRefreshInterval: 5000,
        defaultSessionPrefix: 'shell',
        musicVolume: 0.5,
        musicEnabled: false,
        tmuxAppearance: {
          statusBg: 'default',
          statusFg: '#00ff41',
          paneBorderActive: '#00ff41',
          paneBorderInactive: '#333333',
          modeStyleBg: '#00ff41',
          modeStyleFg: '#000000',
        },
      },
    }

    localStorage.setItem('chrote-dashboard-state', JSON.stringify(v2State))

    const { result } = renderSession()

    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alpha'])
    expect(result.current.workspaces.terminal2.windows[0].boundSessions).toEqual(['beta'])
    expect(result.current.workspaces.terminal2.windowCount).toBe(2)
    expect(result.current.workspaces.terminal1.windows).toHaveLength(4)
    expect(result.current.workspaces.terminal2.windows).toHaveLength(4)
  })

  it('returns defaults for empty/corrupt localStorage', () => {
    localStorage.setItem('chrote-dashboard-state', '{invalid json')

    const { result } = renderSession()

    // Should get clean defaults — 2 windows per workspace, all empty
    expect(result.current.workspaces.terminal1.windowCount).toBe(2)
    expect(result.current.workspaces.terminal2.windowCount).toBe(2)
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual([])
  })

  it('keeps window layouts separate between desktop and mobile viewports', () => {
    setViewportWidth(1280)
    const desktop = renderSession()

    act(() => {
      desktop.result.current.setWindowCount('terminal1', 4)
    })

    expect(desktop.result.current.workspaces.terminal1.windowCount).toBe(4)
    desktop.unmount()

    setViewportWidth(390)
    const mobile = renderSession()

    expect(mobile.result.current.workspaces.terminal1.windowCount).toBe(2)

    act(() => {
      mobile.result.current.setWindowCount('terminal1', 1)
    })

    expect(mobile.result.current.workspaces.terminal1.windowCount).toBe(1)
    mobile.unmount()

    setViewportWidth(1280)
    const desktopReload = renderSession()

    expect(desktopReload.result.current.workspaces.terminal1.windowCount).toBe(4)
  })
})

// ──────────────────────────────────────────────
// 3b. live viewport bucket switching (ctx-00t)
// ──────────────────────────────────────────────
describe('live viewport bucket switching', () => {
  beforeEach(() => {
    localStorage.clear()
    setViewportWidth(1280)
  })

  function crossTo(width: number) {
    act(() => {
      setViewportWidth(width)
      window.dispatchEvent(new Event('resize'))
    })
  }

  function persisted() {
    return JSON.parse(localStorage.getItem('chrote-dashboard-state') ?? '{}')
  }

  it('carries the current layout into a never-used bucket and persists later edits under the new key only', () => {
    const hook = renderSession()
    act(() => { hook.result.current.setWindowCount('terminal1', 4) })

    crossTo(390)

    expect(hook.result.current.workspaces.terminal1.windowCount).toBe(4)

    act(() => { hook.result.current.setWindowCount('terminal1', 1) })

    expect(persisted().layoutsByViewport.mobile.workspaces.terminal1.windowCount).toBe(1)
    expect(persisted().layoutsByViewport.desktop.workspaces.terminal1.windowCount).toBe(4)
    hook.unmount()
  })

  it('loads the stored layout live when crossing into a bucket that already has one', () => {
    const hook = renderSession()
    act(() => { hook.result.current.setWindowCount('terminal1', 4) })

    crossTo(390)
    act(() => { hook.result.current.setWindowCount('terminal1', 1) })

    crossTo(1280)

    expect(hook.result.current.workspaces.terminal1.windowCount).toBe(4)
    expect(persisted().layoutsByViewport.mobile.workspaces.terminal1.windowCount).toBe(1)
    expect(persisted().layoutsByViewport.desktop.workspaces.terminal1.windowCount).toBe(4)
    hook.unmount()
  })

  it('survives rapid breakpoint flapping with a consistent final bucket and no cross-key leakage', () => {
    const hook = renderSession()
    act(() => { hook.result.current.setWindowCount('terminal1', 3) })

    crossTo(390)
    crossTo(1000)
    crossTo(1280)
    crossTo(390)

    act(() => { hook.result.current.setWindowCount('terminal1', 1) })

    expect(persisted().layoutsByViewport.mobile.workspaces.terminal1.windowCount).toBe(1)
    expect(persisted().layoutsByViewport.desktop.workspaces.terminal1.windowCount).toBe(3)
    expect(persisted().layoutsByViewport.tablet.workspaces.terminal1.windowCount).toBe(3)
    hook.unmount()
  })

  it('ignores resizes that stay inside the same bucket', () => {
    const hook = renderSession()
    act(() => { hook.result.current.setWindowCount('terminal1', 4) })
    const before = hook.result.current.workspaces

    crossTo(1400)

    expect(hook.result.current.workspaces).toBe(before)
    expect(persisted().layoutsByViewport.mobile).toBeUndefined()
    hook.unmount()
  })

  it('removes the viewport listener on unmount', () => {
    const hook = renderSession()
    hook.unmount()

    expect(() => {
      setViewportWidth(390)
      window.dispatchEvent(new Event('resize'))
    }).not.toThrow()
  })
})

// ──────────────────────────────────────────────
// 4. launch-user resolution
// ──────────────────────────────────────────────
describe('resolveLaunchUser', () => {
  it('keeps default/no configured-users mode as bare tmux sessions even with stale stored launch user settings', () => {
    const settings = {
      ...DEFAULT_SETTINGS,
      terminalLaunchUsers: {
        ...DEFAULT_SETTINGS.terminalLaunchUsers,
        terminal1: 'alice',
      },
    }

    expect(resolveLaunchUser(settings, 'terminal1', [])).toBe('')
  })

  it('uses configured launch users only when the server advertises that user-scoped mode is enabled', () => {
    const settings = {
      ...DEFAULT_SETTINGS,
      terminalLaunchUsers: {
        ...DEFAULT_SETTINGS.terminalLaunchUsers,
        terminal3: 'build',
      },
    }

    expect(resolveLaunchUser(settings, 'terminal3', ['alice', 'build'])).toBe('build')
  })
})

// ──────────────────────────────────────────────
// 5. createSession — single creation path, optional attach
// ──────────────────────────────────────────────
describe('createSession', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  function stubSessionFetch(existingSessions = [
    { name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' },
  ]) {
    const fetchMock = vi.fn((_: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({}),
          text: () => Promise.resolve(''),
        })
      }

      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({
          sessions: existingSessions,
          grouped: {},
          terminalUsers: ['alice', 'build'],
          timestamp: new Date().toISOString(),
        }),
        text: () => Promise.resolve(''),
      })
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  it('creates a standalone side-panel session through the shared action without attaching it', async () => {
    const fetchMock = stubSessionFetch()
    const { result } = renderSession()

    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice', 'build']))
    fetchMock.mockClear()

    let created: string | null = null
    await act(async () => {
      created = await result.current.createSession({ workspaceId: 'terminal1', unixUser: 'alice' })
    })

    expect(created).toBe('shell2')
    expect(fetchMock).toHaveBeenCalled()
    const [, init] = fetchMock.mock.calls[0]
    expect(JSON.parse(String(init?.body))).toEqual({ name: 'shell2', unixUser: 'alice', mouseScroll: true })
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual([])
  })

  it('creates and immediately attaches a window session through the same shared action', async () => {
    const fetchMock = stubSessionFetch()
    const { result } = renderSession()

    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice', 'build']))
    fetchMock.mockClear()

    let created: string | null = null
    await act(async () => {
      created = await result.current.createSession({
        workspaceId: 'terminal3',
        unixUser: 'build',
        attachTo: { workspaceId: 'terminal3', windowId: 'terminal3-window-0' },
      })
    })

    expect(created).toBe('build1')
    const [, init] = fetchMock.mock.calls[0]
    expect(JSON.parse(String(init?.body))).toEqual({ name: 'build1', unixUser: 'build', mouseScroll: true })
    const win = result.current.workspaces.terminal3.windows[0]
    expect(win.boundSessions).toEqual(['build:build1'])
    expect(win.activeSession).toBe('build:build1')
  })

  it('passes saved disabled mouse-scroll preference when creating sessions', async () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {},
      settings: { ...DEFAULT_SETTINGS, mouseScroll: false },
    }))
    const fetchMock = stubSessionFetch()
    const { result } = renderSession()

    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice', 'build']))
    fetchMock.mockClear()

    await act(async () => {
      await result.current.createSession({ workspaceId: 'terminal1', unixUser: 'alice' })
    })

    const [, init] = fetchMock.mock.calls[0]
    expect(JSON.parse(String(init?.body))).toEqual({ name: 'shell2', unixUser: 'alice', mouseScroll: false })
  })

  it('handles expected create-session API failures with a toast and no console error', async () => {
    const fetchMock = stubSessionFetch()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { result } = renderSession()

    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice', 'build']))
    fetchMock.mockClear()
    fetchMock.mockImplementationOnce(() => Promise.resolve({
      ok: false,
      json: () => Promise.resolve({ error: 'tmux not running' }),
      text: () => Promise.resolve('{"error":"tmux not running"}'),
    }))

    let created: string | null = 'not-null'
    await act(async () => {
      created = await result.current.createSession({ workspaceId: 'terminal1', unixUser: 'alice' })
    })

    expect(created).toBeNull()
    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
  })
})

describe('sendToSession', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('resolves exact pane identities for the selected Unix user', async () => {
    const pane = { sessionId: '$7', pane: '%42', panePid: '222', serverPid: '9001', windowName: 'logs', currentCommand: 'tail', currentPath: '/srv/app', active: true }
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/panes')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ success: true, session: 'shell1', unixUser: 'alice', panes: [pane] }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSession()
    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice']))
    fetchMock.mockClear()

    let panes = null
    await act(async () => {
      panes = await result.current.listSessionPanes('shell1', 'alice')
    })

    expect(panes).toEqual([pane])
    expect(fetchMock).toHaveBeenCalledWith('/api/tmux/sessions/shell1/panes?unixUser=alice', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })

  it('rejects pane discovery from a non-empty Unix user when default scope was requested', async () => {
    const pane = { sessionId: '$7', pane: '%42', panePid: '222', serverPid: '9001', active: true }
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/panes')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ success: true, session: 'shell1', unixUser: 'alice', panes: [pane] }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({ sessions: [], terminalUsers: [] }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSession()
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    fetchMock.mockClear()

    let panes: Awaited<ReturnType<typeof result.current.listSessionPanes>> = []
    await act(async () => {
      panes = await result.current.listSessionPanes('shell1')
    })

    expect(panes).toBeNull()
  })

  it('rejects send success from a non-empty Unix user when default scope was requested', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({
            success: true,
            transport: 'pasted',
            session: 'shell1',
            sessionId: '$7',
            pane: '%42',
            panePid: '222',
            serverPid: '9001',
            unixUser: 'alice',
            submissionRequested: false,
            submitKeyDispatched: false,
            bufferCleaned: true,
            targetVerified: true,
            warning: '',
          }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({ sessions: [], terminalUsers: [] }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSessionWithToast()
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    fetchMock.mockClear()

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.session.sendToSession('shell1', { text: 'wrong scope', files: [], submit: false })
    })

    expect(delivered).toBe('unknown')
    await waitFor(() => expect(result.current.toast.toasts[0]?.message).toContain('Unexpected send response'))
  })

  it('keeps a marker-confirmed send delivered when post-send verification observes target exit', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, _init?: RequestInit) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({
            success: true,
            transport: 'pasted',
            session: 'shell1',
            sessionId: '$7',
            pane: '%42',
            panePid: '222',
            serverPid: '9001',
            unixUser: 'alice',
            submissionRequested: true,
            submitKeyDispatched: true,
            bufferCleaned: true,
            targetVerified: false,
            warning: 'target changed before post-send verification',
          }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSession()
    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice']))
    fetchMock.mockClear()

    let delivered: SendToSessionOutcome = 'failed'
    await act(async () => {
      delivered = await result.current.sendToSession('shell1', {
        text: 'inspect this',
        files: [],
        submit: true,
        pane: '%42',
        sessionId: '$7',
        panePid: '222',
        serverPid: '9001',
      }, 'alice')
    })

    expect(delivered).toBe('sent')
    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toBe('/api/tmux/sessions/shell1/send?unixUser=alice')
    const form = init?.body as FormData
    expect(form.get('text')).toBe('inspect this')
    expect(form.get('submit')).toBe('true')
    expect(form.get('pane')).toBe('%42')
    expect(form.get('sessionId')).toBe('$7')
    expect(form.get('panePid')).toBe('222')
    expect(form.get('serverPid')).toBe('9001')
  })

  it('reports a confirmed paste without a submit-key receipt as unknown and non-retryable', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({
            success: true,
            transport: 'pasted',
            session: 'shell1',
            sessionId: '$7',
            pane: '%42',
            panePid: '222',
            serverPid: '9001',
            unixUser: 'alice',
            submissionRequested: true,
            submitKeyDispatched: false,
            bufferCleaned: true,
            targetVerified: false,
            warning: 'target changed after paste; submit key was not dispatched',
          }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSessionWithToast()
    await waitFor(() => expect(result.current.session.terminalUsers).toEqual(['alice']))
    fetchMock.mockClear()

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.session.sendToSession('shell1', {
        text: 'pasted but not submitted',
        files: [],
        submit: true,
        pane: '%42',
        sessionId: '$7',
        panePid: '222',
        serverPid: '9001',
      }, 'alice')
    })

    expect(delivered).toBe('unknown')
    await waitFor(() => expect(result.current.toast.toasts[0]?.message).toContain('submit key was not dispatched'))
  })

  it('treats an explicit unknown delivery outcome as non-retryable and tells the user to inspect', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, _init?: RequestInit) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({
            success: false,
            transport: 'unknown',
            retryable: false,
            deliveryConfirmed: false,
            session: 'shell1',
            sessionId: '$7',
            pane: '%42',
            panePid: '222',
            serverPid: '9001',
            unixUser: 'alice',
            submissionRequested: true,
            submitKeyDispatched: false,
            bufferCleaned: true,
            targetVerified: false,
            warning: 'tmux did not confirm whether delivery occurred; inspect the exact pane before retrying',
          }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSessionWithToast()
    await waitFor(() => expect(result.current.session.terminalUsers).toEqual(['alice']))
    fetchMock.mockClear()

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.session.sendToSession('shell1', {
        text: 'do not duplicate',
        files: [],
        submit: true,
        pane: '%42',
        sessionId: '$7',
        panePid: '222',
        serverPid: '9001',
      }, 'alice')
    })

    expect(delivered).toBe('unknown')
    await waitFor(() => expect(result.current.toast.toasts[0]?.message).toContain('inspect the exact pane before retrying'))
  })

  it('labels browser transport failures as unknown rather than safe-to-retry failures', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.reject(new Error('connection reset after request write'))
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSessionWithToast()
    await waitFor(() => expect(result.current.session.terminalUsers).toEqual(['alice']))

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.session.sendToSession('shell1', {
        text: 'could already be pasted',
        files: [],
        submit: false,
      }, 'alice')
    })

    expect(delivered).toBe('unknown')
    await waitFor(() => expect(result.current.toast.toasts[0]?.message).toContain('inspect the exact pane before retrying'))
  })

  it.each([
    {
      title: 'shows a validated pre-dispatch 400 as an ordinary failure',
      status: 400,
      body: JSON.stringify({ success: false, error: { code: 'BAD_REQUEST', message: 'text or files are required' }, timestamp: '2026-07-18T18:00:00Z' }),
      expected: 'text or files are required',
      unknown: false,
    },
    {
      title: 'treats a valid-envelope 500 as an unknown delivery outcome',
      status: 500,
      body: JSON.stringify({ success: false, error: { code: 'TMUX_ERROR', message: 'internal tmux failure' }, timestamp: '2026-07-18T18:00:00Z' }),
      expected: 'inspect the exact pane before retrying',
      unknown: true,
    },
    {
      title: 'treats a malformed 409 envelope as an unknown delivery outcome',
      status: 409,
      body: JSON.stringify({ error: { code: 'TARGET_CHANGED', message: 'target changed' } }),
      expected: 'inspect the exact pane before retrying',
      unknown: true,
    },
  ])('$title', async ({ status, body, expected, unknown }) => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: false,
          status,
          text: () => Promise.resolve(body),
          json: () => Promise.reject(new Error('not a success response')),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSessionWithToast()
    await waitFor(() => expect(result.current.session.terminalUsers).toEqual(['alice']))

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.session.sendToSession('shell1', {
        text: 'classify this response',
        files: [],
        submit: false,
      }, 'alice')
    })

    expect(delivered).toBe(unknown ? 'unknown' : 'failed')
    await waitFor(() => expect(result.current.toast.toasts[0]?.message).toContain(expected))
    if (!unknown) {
      expect(result.current.toast.toasts[0]?.message).not.toContain('outcome is unknown')
    }
  })

  it('rejects a malformed 2xx transport response instead of claiming delivery', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, _init?: RequestInit) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ success: true, transport: 'queued', pane: '%42' }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSession()
    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice']))
    fetchMock.mockClear()

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.sendToSession('shell1', {
        text: 'do not overclaim',
        files: [],
        submit: false,
        pane: '%42',
        sessionId: '$7',
        panePid: '222',
        serverPid: '9001',
      }, 'alice')
    })

    expect(delivered).toBe('unknown')
  })
})


describe('refreshSessions', () => {
  beforeEach(() => {
    localStorage.clear()
    setViewportWidth(1280)
    vi.useRealTimers()
  })

  it('runs one request at a time and coalesces interval, manual, create, delete, and rename triggers', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const { result } = renderSession()

    expect(requests).toHaveLength(1)
    const refreshSessions = result.current.refreshSessions
    let coalescedSettled = false

    act(() => {
      void result.current.refreshSessions().then(() => { coalescedSettled = true })
      void result.current.refreshSessions()
      vi.advanceTimersByTime(DEFAULT_SETTINGS.autoRefreshInterval)
    })
    await act(async () => {
      await Promise.all([
        result.current.createSession({ name: 'created' }),
        result.current.deleteSession('deleted'),
        result.current.renameSession('old', 'renamed'),
      ])
    })

    expect(result.current.refreshSessions).toBe(refreshSessions)
    expect(requests).toHaveLength(1)

    await act(async () => {
      requests[0].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: ['alice'] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(2)
    expect(coalescedSettled).toBe(false)

    act(() => {
      void result.current.refreshSessions()
      void result.current.refreshSessions()
      vi.advanceTimersByTime(DEFAULT_SETTINGS.autoRefreshInterval)
    })
    expect(requests).toHaveLength(2)

    await act(async () => {
      requests[1].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: ['alice'] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(3)
    expect(coalescedSettled).toBe(true)

    await act(async () => {
      requests[2].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: ['alice'] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(3)
    vi.useRealTimers()
  })

  it.each([
    ['createSession', 'POST', '/api/tmux/sessions'],
    ['deleteSession', 'DELETE', '/api/tmux/sessions/deleted'],
    ['renameSession', 'PATCH', '/api/tmux/sessions/old'],
  ] as const)('%s alone requests one trailing GET while the initial GET is unresolved', async (actionName, method, url) => {
    vi.useFakeTimers()
    const { fetchMock, requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()

    expect(requests).toHaveLength(1)

    await act(async () => {
      if (actionName === 'createSession') {
        expect(await result.current.createSession({ name: 'created' })).toBe('created')
      } else if (actionName === 'deleteSession') {
        expect(await result.current.deleteSession('deleted')).toBe(true)
      } else {
        expect(await result.current.renameSession('old', 'renamed')).toBe(true)
      }
    })

    expect(fetchMock.mock.calls.filter(([input, init]) => String(input) === url && init?.method === method)).toHaveLength(1)
    expect(requests).toHaveLength(1)

    await act(async () => {
      requests[0].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: [] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(2)

    await act(async () => {
      requests[1].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: [] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(2)

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it('keeps a refresh requested during the trailing flight pending through one further coalesced GET', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()

    expect(requests).toHaveLength(1)
    act(() => {
      void result.current.refreshSessions()
    })

    await act(async () => {
      requests[0].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: [] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(2)

    let refreshSettled = false
    let refreshDuringTrailing!: Promise<void>
    act(() => {
      refreshDuringTrailing = result.current.refreshSessions()
      void refreshDuringTrailing.then(() => { refreshSettled = true })
    })
    expect(requests).toHaveLength(2)
    expect(refreshSettled).toBe(false)

    await act(async () => {
      requests[1].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: [] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(3)
    expect(refreshSettled).toBe(false)

    await act(async () => {
      requests[2].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: [] }))
      await refreshDuringTrailing
    })
    expect(refreshSettled).toBe(true)
    expect(requests).toHaveLength(3)

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it.each(['fetch', 'response.json'] as const)(
    'times out when %s ignores abort, starts the queued trailing refresh, and remains reusable',
    async stalledStage => {
      vi.useFakeTimers()
      const { requests } = stubDeferredSessionFetch()
      const { result, unmount } = renderSession()

      expect(requests).toHaveLength(1)
      if (stalledStage === 'response.json') {
        const neverSettlingBody = new Promise<never>(() => {})
        const stalledJson = vi.fn(() => neverSettlingBody)
        await act(async () => {
          requests[0].response.resolve({
            ok: true,
            json: stalledJson,
            text: vi.fn(() => Promise.resolve('')),
          })
          await flushPromises()
        })
        expect(stalledJson).toHaveBeenCalledOnce()
      }

      let queuedSettled = false
      let queuedRefresh!: Promise<void>
      act(() => {
        queuedRefresh = result.current.refreshSessions()
        void queuedRefresh.then(() => { queuedSettled = true })
      })

      await act(async () => {
        vi.advanceTimersByTime(10000)
        await Promise.resolve()
      })

      expect(requests[0].signal?.aborted).toBe(true)
      expect(requests).toHaveLength(2)
      expect(queuedSettled).toBe(false)
      expect(result.current.loading).toBe(false)
      // One polling interval plus the trailing flight timeout: the abandoned flight timer is gone.
      expect(vi.getTimerCount()).toBe(2)

      await act(async () => {
        requests[1].response.resolve(sessionResponse({
          sessions: [{ name: 'trailing' }],
          grouped: {},
          banked: [],
          terminalUsers: [],
        }))
        await queuedRefresh
      })
      expect(queuedSettled).toBe(true)
      expect(result.current.sessions).toEqual([{ name: 'trailing' }])
      expect(vi.getTimerCount()).toBe(1)

      const laterRefresh = result.current.refreshSessions()
      expect(requests).toHaveLength(3)
      await act(async () => {
        requests[2].response.resolve(sessionResponse({
          sessions: [{ name: 'later' }],
          grouped: {},
          banked: [],
          terminalUsers: [],
        }))
        await laterRefresh
      })
      expect(result.current.sessions).toEqual([{ name: 'later' }])
      expect(result.current.loading).toBe(false)
      expect(vi.getTimerCount()).toBe(1)

      unmount()
      expect(vi.getTimerCount()).toBe(0)
      vi.useRealTimers()
    },
  )

  it('ignores an eventual rejection from an abandoned response body after a newer refresh succeeds', async () => {
    vi.useFakeTimers()
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {},
      settings: { ...DEFAULT_SETTINGS, autoRefreshInterval: 60000 },
    }))
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()
    const abandonedBody = deferred<Record<string, unknown>>()
    const abandonedJson = vi.fn(() => abandonedBody.promise)
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    await act(async () => {
      requests[0].response.resolve({
        ok: true,
        json: abandonedJson,
        text: vi.fn(() => Promise.resolve('')),
      })
      await flushPromises()
    })
    expect(abandonedJson).toHaveBeenCalledOnce()
    await act(async () => {
      vi.advanceTimersByTime(10000)
      await flushPromises()
    })

    let newerSettled = false
    const newerRefresh = result.current.refreshSessions()
    void newerRefresh.then(() => { newerSettled = true })
    expect(requests).toHaveLength(2)
    await act(async () => {
      requests[1].response.resolve(sessionResponse({
        sessions: [{ name: 'newer' }],
        grouped: {},
        banked: [],
        terminalUsers: [],
      }))
      await flushPromises()
    })
    expect(newerSettled).toBe(true)

    await act(async () => {
      abandonedBody.reject(new Error('late body failure'))
      await Promise.resolve()
    })
    expect(result.current.sessions).toEqual([{ name: 'newer' }])
    expect(result.current.error).toBeNull()
    expect(consoleError).not.toHaveBeenCalled()

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    consoleError.mockRestore()
    vi.useRealTimers()
  })

  it('settles coalesced callers on unmount without resolving an abort-ignoring fetch or honoring trailing intent', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    let settledCallers = 0

    act(() => {
      void result.current.refreshSessions().then(() => { settledCallers += 1 })
      void result.current.refreshSessions().then(() => { settledCallers += 1 })
    })
    expect(requests).toHaveLength(1)

    unmount()
    expect(requests[0].signal?.aborted).toBe(true)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(settledCallers).toBe(2)
    expect(requests).toHaveLength(1)
    expect(vi.getTimerCount()).toBe(0)
    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
    vi.useRealTimers()
  })

  it('keeps StrictMode generation cancellation silent and ignores its eventual completion', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const wrapper = ({ children }: { children: React.ReactNode }) => createElement(
      StrictMode,
      null,
      createElement(Wrapper, null, children),
    )
    const { result, unmount } = renderHook(() => useSession(), { wrapper })

    expect(requests).toHaveLength(2)
    expect(requests[0].signal?.aborted).toBe(true)
    expect(requests[1].signal?.aborted).toBe(false)

    await act(async () => {
      requests[1].response.resolve(sessionResponse({
        sessions: [{ name: 'current-generation' }],
        grouped: {},
        banked: [],
        terminalUsers: [],
      }))
      await flushPromises()
    })
    expect(result.current.sessions).toEqual([{ name: 'current-generation' }])
    expect(result.current.error).toBeNull()

    const abandonedResponse = sessionResponse({ sessions: [{ name: 'abandoned-generation' }] })
    await act(async () => {
      requests[0].response.resolve(abandonedResponse)
      await flushPromises()
    })
    expect(abandonedResponse.json).not.toHaveBeenCalled()
    expect(result.current.sessions).toEqual([{ name: 'current-generation' }])
    expect(result.current.error).toBeNull()
    expect(consoleError).not.toHaveBeenCalled()

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    consoleError.mockRestore()
    vi.useRealTimers()
  })

  it('reports a mounted timeout while preserving known state, bindings, modals, and stale confirmation', async () => {
    vi.useFakeTimers()
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [{
                id: 'terminal1-window-0',
                boundSessions: ['alice:known'],
                activeSession: 'alice:known',
                colorIndex: 0,
              }],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: false,
      settings: { ...DEFAULT_SETTINGS, autoRefreshInterval: 60000 },
    }))
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()
    const knownSession = { name: 'known', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }
    const knownGrouped = { shell: [knownSession] }
    const knownBank = [{ name: 'banked', unixUser: 'alice', live: false }]

    await act(async () => {
      requests[0].response.resolve(sessionResponse({
        sessions: [knownSession],
        grouped: knownGrouped,
        banked: knownBank,
        terminalUsers: ['alice'],
      }))
      await flushPromises()
    })
    expect(result.current.sessions).toEqual([knownSession])
    act(() => {
      result.current.openFloatingModal('alice:known')
      result.current.openSendToSession('alice:known')
    })

    let timeoutSettled = false
    let timedOutRefresh!: Promise<void>
    act(() => {
      timedOutRefresh = result.current.refreshSessions()
      void timedOutRefresh.then(() => { timeoutSettled = true })
    })
    expect(requests).toHaveLength(2)

    await act(async () => {
      vi.advanceTimersByTime(10000)
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(requests[1].signal?.aborted).toBe(true)
    expect(timeoutSettled).toBe(true)
    await timedOutRefresh
    expect(requests).toHaveLength(2)
    expect(result.current).toMatchObject({
      sessions: [knownSession],
      groupedSessions: knownGrouped,
      sessionBank: knownBank,
      terminalUsers: ['alice'],
      floatingSession: 'alice:known',
      sendToSessionTarget: 'alice:known',
      loading: false,
      error: 'Failed to fetch sessions (request timed out)',
    })
    expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
      boundSessions: ['alice:known'],
      activeSession: 'alice:known',
    })
    expect(vi.getTimerCount()).toBe(1)

    const firstAuthoritativeMissing = result.current.refreshSessions()
    expect(requests).toHaveLength(3)
    await act(async () => {
      requests[2].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: [] }))
      await firstAuthoritativeMissing
    })
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:known'])
    expect(result.current.floatingSession).toBe('alice:known')
    expect(result.current.sendToSessionTarget).toBe('alice:known')

    const secondAuthoritativeMissing = result.current.refreshSessions()
    await act(async () => {
      requests[3].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: [] }))
      await secondAuthoritativeMissing
    })
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:known'])
    expect(result.current.floatingSession).toBe('alice:known')
    expect(result.current.sendToSessionTarget).toBe('alice:known')

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it('replaces last-known-good state with healthy sessions from an authoritative partial refresh and keeps the per-user error visible', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()
    const knownSession = { name: 'known', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }
    const healthySession = { name: 'healthy', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }

    await act(async () => {
      requests[0].response.resolve(sessionResponse({
        sessions: [knownSession],
        grouped: { shell: [knownSession] },
        banked: [{ name: 'known-banked', unixUser: 'alice', live: false }],
        terminalUsers: ['alice'],
      }))
      await flushPromises()
    })
    expect(result.current.sessions).toEqual([knownSession])

    const partialRefresh = result.current.refreshSessions()
    await act(async () => {
      requests[1].response.resolve(sessionResponse({
        partial: true,
        error: 'build: error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)',
        sessions: [healthySession],
        grouped: { shell: [healthySession] },
        banked: [],
        terminalUsers: ['alice', 'build'],
      }))
      await partialRefresh
    })

    expect(result.current.sessions).toEqual([healthySession])
    expect(result.current.groupedSessions).toEqual({ shell: [healthySession] })
    expect(result.current.terminalUsers).toEqual(['alice', 'build'])
    expect(result.current.error).toBe('build: error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)')

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it('preserves authoritative state and stale confirmation across failed, non-ok, and aborted refreshes', async () => {
    vi.useFakeTimers()
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [{
                id: 'terminal1-window-0',
                boundSessions: ['alice:stale'],
                activeSession: 'alice:stale',
                colorIndex: 0,
              }],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: false,
      settings: { ...DEFAULT_SETTINGS, autoRefreshInterval: 60000 },
    }))
    const { requests } = stubDeferredSessionFetch()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { result } = renderSession()
    const staleSession = { name: 'stale', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }
    const knownBank = [{ name: 'banked', unixUser: 'alice', live: false }]

    await act(async () => {
      requests[0].response.resolve(sessionResponse({
        sessions: [staleSession],
        grouped: { shell: [staleSession] },
        banked: knownBank,
        terminalUsers: ['alice'],
      }))
      await Promise.resolve()
    })
    act(() => {
      result.current.openFloatingModal('alice:stale')
      result.current.openSendToSession('alice:stale')
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'protected', 'alice')
    })

    const knownState = {
      sessions: result.current.sessions,
      groupedSessions: result.current.groupedSessions,
      sessionBank: result.current.sessionBank,
      terminalUsers: result.current.terminalUsers,
      floatingSession: result.current.floatingSession,
      sendToSessionTarget: result.current.sendToSessionTarget,
    }
    const failedPayload = {
      error: 'tmux unavailable',
      sessions: [{ name: 'not-authoritative' }],
      grouped: { bad: [{ name: 'not-authoritative' }] },
      banked: [{ name: 'not-authoritative' }],
      terminalUsers: ['not-authoritative'],
    }

    const nonOk = result.current.refreshSessions()
    await act(async () => {
      requests[1].response.resolve(sessionResponse(failedPayload, false))
      await nonOk
    })
    expect(result.current).toMatchObject(knownState)

    const failed = result.current.refreshSessions()
    await act(async () => {
      requests[2].response.resolve(sessionResponse(failedPayload))
      await failed
    })
    expect(result.current).toMatchObject(knownState)

    const aborted = result.current.refreshSessions()
    expect(requests[3].signal?.aborted).toBe(false)
    act(() => vi.advanceTimersByTime(10000))
    expect(requests[3].signal?.aborted).toBe(true)
    await act(async () => {
      requests[3].response.reject(new DOMException('Aborted', 'AbortError'))
      await aborted
    })
    expect(result.current).toMatchObject(knownState)
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:stale', 'alice:protected'])

    const firstMissing = result.current.refreshSessions()
    await act(async () => {
      requests[4].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: ['alice'] }))
      await firstMissing
    })
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:stale', 'alice:protected'])
    expect(result.current.floatingSession).toBe('alice:stale')
    expect(result.current.sendToSessionTarget).toBe('alice:stale')

    const secondMissing = result.current.refreshSessions()
    await act(async () => {
      requests[5].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: ['alice'] }))
      await secondMissing
    })
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:stale', 'alice:protected'])
    expect(result.current.floatingSession).toBe('alice:stale')
    expect(result.current.sendToSessionTarget).toBe('alice:stale')

    for (const requestIndex of [6, 7]) {
      const refresh = result.current.refreshSessions()
      await act(async () => {
        requests[requestIndex].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: ['alice'] }))
        await refresh
      })
    }
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:stale', 'alice:protected'])

    consoleError.mockRestore()
    vi.useRealTimers()
  })

  it.each(['invalid JSON', 'a non-aborted network rejection'] as const)(
    'preserves all authoritative state and stale confirmation after %s',
    async failureMode => {
      vi.useFakeTimers()
      localStorage.setItem('chrote-dashboard-state', JSON.stringify({
        version: 3,
        settingsSchemaVersion: 2,
        layoutsByViewport: {
          desktop: {
            workspaces: {
              terminal1: {
                windows: [{
                  id: 'terminal1-window-0',
                  boundSessions: ['alice:known'],
                  activeSession: 'alice:known',
                  colorIndex: 0,
                }],
                windowCount: 1,
              },
            },
          },
        },
        sidebarCollapsed: false,
        settings: { ...DEFAULT_SETTINGS, autoRefreshInterval: 60000 },
      }))
      const { requests } = stubDeferredSessionFetch()
      const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
      const { result, unmount } = renderSession()
      const knownSession = { name: 'known', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }
      const knownGrouped = { shell: [knownSession] }
      const knownBank = [{ name: 'banked', unixUser: 'alice', live: false }]

      await act(async () => {
        requests[0].response.resolve(sessionResponse({
          sessions: [knownSession],
          grouped: knownGrouped,
          banked: knownBank,
          terminalUsers: ['alice'],
        }))
        await Promise.resolve()
      })
      act(() => {
        result.current.openFloatingModal('alice:known')
        result.current.openSendToSession('alice:known')
      })

      const failedRefresh = result.current.refreshSessions()
      expect(requests).toHaveLength(2)
      expect(requests[1].signal?.aborted).toBe(false)
      await act(async () => {
        if (failureMode === 'invalid JSON') {
          requests[1].response.resolve({
            ok: true,
            json: vi.fn(() => Promise.reject(new SyntaxError('Unexpected token'))),
            text: vi.fn(() => Promise.resolve('not json')),
          })
        } else {
          requests[1].response.reject(new TypeError('network unavailable'))
        }
        await failedRefresh
      })

      expect(result.current).toMatchObject({
        sessions: [knownSession],
        groupedSessions: knownGrouped,
        sessionBank: knownBank,
        terminalUsers: ['alice'],
        floatingSession: 'alice:known',
        sendToSessionTarget: 'alice:known',
      })
      expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
        boundSessions: ['alice:known'],
        activeSession: 'alice:known',
      })

      const firstMissing = result.current.refreshSessions()
      await act(async () => {
        requests[2].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: [] }))
        await firstMissing
      })
      expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
        boundSessions: ['alice:known'],
        activeSession: 'alice:known',
      })
      expect(result.current.floatingSession).toBe('alice:known')
      expect(result.current.sendToSessionTarget).toBe('alice:known')

      const secondMissing = result.current.refreshSessions()
      await act(async () => {
        requests[3].response.resolve(sessionResponse({ sessions: [], grouped: {}, banked: [], terminalUsers: [] }))
        await secondMissing
      })
      expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
        boundSessions: ['alice:known'],
        activeSession: 'alice:known',
      })
      expect(result.current.floatingSession).toBe('alice:known')
      expect(result.current.sendToSessionTarget).toBe('alice:known')

      unmount()
      expect(vi.getTimerCount()).toBe(0)
      consoleError.mockRestore()
      vi.useRealTimers()
    },
  )

  it('preserves offline terminal placement until the operator explicitly resolves it', async () => {
    const liveSessions = [
      { name: 'alive', windows: 1, attached: false, group: 'shell', unixUser: 'alice' },
      { name: 'legacy-live', windows: 1, attached: false, group: 'shell' },
      { name: 'agent-live', windows: 1, attached: false, group: 'agents', unixUser: 'build' },
    ]
    const banked = [
      {
        name: 'gone', windows: 1, attached: false, group: 'shell', unixUser: 'alice', live: false,
        firstSeen: '2026-07-10T10:00:00Z', lastSeen: '2026-07-10T10:05:00Z', recoveryKind: 'shell',
      },
    ]
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [
                {
                  id: 'terminal1-window-0',
                  boundSessions: ['alice:alive', 'alice:gone', 'legacy-live', 'legacy-gone'],
                  activeSession: 'alice:gone',
                  colorIndex: 0,
                },
              ],
              windowCount: 1,
            },
            terminal2: {
              windows: [
                {
                  id: 'terminal2-window-0',
                  boundSessions: ['build:agent-live', 'build:agent-dead'],
                  activeSession: 'build:agent-dead',
                  colorIndex: 0,
                },
              ],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: false,
      settings: DEFAULT_SETTINGS,
    }))
    const fetchMock = vi.fn((): Promise<any> => Promise.resolve({
      ok: true,
      json: () => Promise.resolve({
        sessions: liveSessions,
        grouped: { shell: liveSessions.slice(0, 2), agents: liveSessions.slice(2) },
        banked,
        recoveryEvidence: [
          { sourceId: 'tmux:alice', unixUser: 'alice', name: 'gone', state: 'offline', cwd: '/srv/chrote' },
          { sourceId: 'tmux:alice', unixUser: 'alice', name: 'legacy-gone', state: 'offline' },
          { sourceId: 'tmux:build', unixUser: 'build', name: 'agent-dead', state: 'offline' },
        ],
        timestamp: new Date().toISOString(),
      }),
      text: () => Promise.resolve(''),
    }))
    vi.stubGlobal('fetch', fetchMock)

    const { result } = renderSession()

    await waitFor(() => expect(result.current.sessions).toEqual(liveSessions))
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toContain('alice:gone')

    await act(async () => {
      await result.current.refreshSessions()
    })

    await waitFor(() => {
      expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:alive', 'alice:gone', 'legacy-live', 'legacy-gone'])
      expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('alice:gone')
      expect(result.current.workspaces.terminal2.windows[0].boundSessions).toEqual(['build:agent-live', 'build:agent-dead'])
      expect(result.current.workspaces.terminal2.windows[0].activeSession).toBe('build:agent-dead')
    })
    expect(result.current.sessionBank).toEqual(banked)
    expect(result.current.recoveryEvidence).toHaveLength(3)
  })

  it('stores managed status registry entries separately from banked sessions', async () => {
    const banked = [{ name: 'banked-agent', unixUser: 'alice', live: false }]
    const managed = [{
      name: 'systemd-worker',
      sessionName: 'systemd-worker',
      unixUser: 'alice',
      owner: { kind: 'external_manager', ref: 'systemd:user/worker.service', mayRestart: false },
      managerKind: 'systemd-user',
      managerRef: 'worker.service',
      status: { ok: true, activeState: 'active', checkedAt: '2026-07-15T10:00:00Z' },
      storageKind: 'managed-status',
      sourceKind: 'restore',
    }]
    const fetchMock = vi.fn((): Promise<any> => Promise.resolve({
      ok: true,
      json: () => Promise.resolve({
        sessions: [],
        grouped: {},
        banked,
        managed,
        terminalUsers: ['alice'],
        timestamp: new Date().toISOString(),
      }),
      text: () => Promise.resolve(''),
    }))
    vi.stubGlobal('fetch', fetchMock)

    const { result } = renderSession()

    await waitFor(() => expect(result.current.sessionBank).toEqual(banked))
    expect(result.current.managedSessions).toEqual(managed)
    expect(result.current.sessionBank).not.toContainEqual(expect.objectContaining({ name: 'systemd-worker' }))
  })

  it('preserves terminal bindings when a refresh fails instead of sweeping on uncertainty', async () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [
                { id: 'terminal1-window-0', boundSessions: ['probably-live'], activeSession: 'probably-live', colorIndex: 0 },
              ],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: false,
      settings: DEFAULT_SETTINGS,
    }))
    vi.stubGlobal('fetch', vi.fn((): Promise<any> => Promise.resolve({
      ok: false,
      json: () => Promise.resolve({ error: 'tmux server not running' }),
      text: () => Promise.resolve('{"error":"tmux server not running"}'),
    })))

    const { result } = renderSession()

    await waitFor(() => expect(result.current.error).toBe('tmux server not running'))
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['probably-live'])
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('probably-live')
  })

  it('preserves existing sessions and groups when a poll returns a non-ok response', async () => {
    const existingSessions = [
      { name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' },
    ]
    const existingGrouped = { shell: existingSessions }
    const fetchMock = vi.fn((): Promise<any> => Promise.resolve({
      ok: true,
      json: () => Promise.resolve({
        sessions: existingSessions,
        grouped: existingGrouped,
        terminalUsers: ['alice'],
        timestamp: new Date().toISOString(),
      }),
      text: () => Promise.resolve(''),
    }))
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSession()

    await waitFor(() => expect(result.current.sessions).toEqual(existingSessions))
    fetchMock.mockImplementationOnce(() => Promise.resolve({
      ok: false,
      json: () => Promise.resolve({ error: 'tmux server not running' }),
      text: () => Promise.resolve('{"error":"tmux server not running"}'),
    }))

    await act(async () => {
      await result.current.refreshSessions()
    })

    expect(result.current.error).toBe('tmux server not running')
    expect(result.current.sessions).toEqual(existingSessions)
    expect(result.current.groupedSessions).toEqual(existingGrouped)
  })
})

// ──────────────────────────────────────────────
// 6. addSessionToWindow cross-window dedup behavior
// ──────────────────────────────────────────────
describe('addSessionToWindow', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('adds a session to an empty target window', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'my-session')
    })

    const win = result.current.workspaces.terminal1.windows[0]
    expect(win.boundSessions).toContain('my-session')
    expect(win.activeSession).toBe('my-session')
  })

  it('removes session from source window before adding to target (cross-workspace dedup)', () => {
    const { result } = renderSession()

    // Bind to terminal1 window-0
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'traveler')
    })
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toContain('traveler')

    // Now move to terminal2 window-0 — should be removed from terminal1
    act(() => {
      result.current.addSessionToWindow('terminal2', 'terminal2-window-0', 'traveler')
    })

    expect(result.current.workspaces.terminal2.windows[0].boundSessions).toContain('traveler')
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).not.toContain('traveler')
  })

  it('keeps same-named sessions from different Unix users distinct', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'shell', 'alice')
    })
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'shell', 'build')
    })

    const win = result.current.workspaces.terminal1.windows[0]
    expect(win.boundSessions).toEqual(['alice:shell', 'build:shell'])
    expect(result.current.assignedSessions.get('alice:shell')).toMatchObject({ workspaceId: 'terminal1', windowId: 'terminal1-window-0' })
    expect(result.current.assignedSessions.get('build:shell')).toMatchObject({ workspaceId: 'terminal1', windowId: 'terminal1-window-0' })
  })

  it('replaces a legacy bare binding with the user-qualified binding instead of duplicating it', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'shell')
    })
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'shell', 'alice')
    })

    const win = result.current.workspaces.terminal1.windows[0]
    expect(win.boundSessions).toEqual(['alice:shell'])
    expect(win.activeSession).toBe('alice:shell')
  })

  it('moves the single safe bare-qualified identity into a non-empty target and activates it', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'shell', 'alice')
      result.current.addSessionToWindow('terminal2', 'terminal2-window-1', 'resident')
    })
    act(() => {
      result.current.addSessionToWindow('terminal2', 'terminal2-window-1', 'shell')
    })

    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual([])
    expect(result.current.workspaces.terminal2.windows[1]).toMatchObject({
      boundSessions: ['resident', 'shell'],
      activeSession: 'shell',
    })
  })

  it('keeps a bare same-name binding distinct when multiple qualified users make the alias ambiguous', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'shell', 'alice')
      result.current.addSessionToWindow('terminal1', 'terminal1-window-1', 'shell', 'bob')
      result.current.addSessionToWindow('terminal2', 'terminal2-window-0', 'shell')
    })

    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:shell'])
    expect(result.current.workspaces.terminal1.windows[1].boundSessions).toEqual(['bob:shell'])
    expect(result.current.workspaces.terminal2.windows[0]).toMatchObject({
      boundSessions: ['shell'],
      activeSession: 'shell',
    })
  })

  it('removes session from another window in the SAME workspace', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'jumper')
    })

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-1', 'jumper')
    })

    expect(result.current.workspaces.terminal1.windows[0].boundSessions).not.toContain('jumper')
    expect(result.current.workspaces.terminal1.windows[1].boundSessions).toContain('jumper')
  })

  it('activates every deliberate attach, including a non-empty destination used by drop and context Attach', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'first')
    })
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('first')

    // Drop/context Attach both use this action, so the newly attached session is visible immediately.
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'second')
    })
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('second')
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['first', 'second'])
  })

  it('when removing dedup source that was activeSession, falls back to first remaining', () => {
    const { result } = renderSession()

    // Bind two sessions to terminal1 window-0, with 'alpha' active
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'alpha')
    })
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'beta')
    })
    act(() => result.current.setActiveSession('terminal1', 'terminal1-window-0', 'alpha'))
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('alpha')

    // Move 'alpha' to terminal2 — source should fall back to 'beta'
    act(() => {
      result.current.addSessionToWindow('terminal2', 'terminal2-window-0', 'alpha')
    })

    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('beta')
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['beta'])
  })
})

// ──────────────────────────────────────────────
// 3. removeSessionFromWindow — active session fallback
// ──────────────────────────────────────────────
describe('removeSessionFromWindow', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('removes the session from the bound list', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'doomed')
    })

    act(() => {
      result.current.removeSessionFromWindow('terminal1', 'terminal1-window-0', 'doomed')
    })

    expect(result.current.workspaces.terminal1.windows[0].boundSessions).not.toContain('doomed')
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBeNull()
  })

  it('when removing the active session, falls back to first remaining session', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'A')
    })
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'B')
    })
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'C')
    })

    // Each deliberate attach activates its session, so select A before testing fallback.
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('C')
    act(() => result.current.setActiveSession('terminal1', 'terminal1-window-0', 'A'))

    // Remove A — should fall back to B (first remaining)
    act(() => {
      result.current.removeSessionFromWindow('terminal1', 'terminal1-window-0', 'A')
    })

    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('B')
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['B', 'C'])
  })

  it('removing a non-active session does not change activeSession', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'X')
    })
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'Y')
    })
    act(() => result.current.setActiveSession('terminal1', 'terminal1-window-0', 'X'))

    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('X')

    act(() => {
      result.current.removeSessionFromWindow('terminal1', 'terminal1-window-0', 'Y')
    })

    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('X')
  })

  it('removing the last session sets activeSession to null', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'only')
    })

    act(() => {
      result.current.removeSessionFromWindow('terminal1', 'terminal1-window-0', 'only')
    })

    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual([])
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBeNull()
  })
})

// ──────────────────────────────────────────────
// 4. cycleSession — wraps in both directions
// ──────────────────────────────────────────────
describe('cycleSession', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  function setupThreeSessions() {
    const hook = renderSession()
    act(() => {
      hook.result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'S0')
    })
    act(() => {
      hook.result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'S1')
    })
    act(() => {
      hook.result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'S2')
    })
    // Deliberate attaches activate their targets; restore the baseline under test.
    act(() => hook.result.current.setActiveSession('terminal1', 'terminal1-window-0', 'S0'))
    return hook
  }

  it('cycles forward through sessions', () => {
    const { result } = setupThreeSessions()
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('S0')

    act(() => result.current.cycleSession('terminal1', 'terminal1-window-0', 'next'))
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('S1')

    act(() => result.current.cycleSession('terminal1', 'terminal1-window-0', 'next'))
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('S2')
  })

  it('wraps forward: next from last goes to first', () => {
    const { result } = setupThreeSessions()

    // Set active to last session
    act(() => result.current.setActiveSession('terminal1', 'terminal1-window-0', 'S2'))
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('S2')

    act(() => result.current.cycleSession('terminal1', 'terminal1-window-0', 'next'))
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('S0')
  })

  it('wraps backward: prev from first goes to last', () => {
    const { result } = setupThreeSessions()
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('S0')

    act(() => result.current.cycleSession('terminal1', 'terminal1-window-0', 'prev'))
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('S2')
  })

  it('does nothing when window has 0 or 1 sessions', () => {
    const { result } = renderSession()

    // Add a single session
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'solo')
    })

    act(() => result.current.cycleSession('terminal1', 'terminal1-window-0', 'next'))
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('solo')

    act(() => result.current.cycleSession('terminal1', 'terminal1-window-0', 'prev'))
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('solo')
  })
})

// ──────────────────────────────────────────────
// 5. renameSession — updates bindings across ALL workspaces
// ──────────────────────────────────────────────
describe('renameSession', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.mocked(fetch as any).mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ sessions: [], grouped: {}, timestamp: new Date().toISOString() }),
      text: () => Promise.resolve(''),
    })
  })

  it('updates all window bindings across all workspaces to use the new name', async () => {
    const { result } = renderSession()

    // Bind same session in terminal1 and terminal2
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'old-name')
    })

    // Add another copy to terminal2 (this first removes from terminal1 due to dedup)
    // So instead, put different sessions in each workspace
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'old-name')
    })

    // For cross-workspace test, we need to seed localStorage directly with the session in both workspaces
    // Because addSessionToWindow deduplicates. Instead, test single workspace rename thoroughly.

    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('old-name')

    await act(async () => {
      const success = await result.current.renameSession('old-name', 'new-name')
      expect(success).toBe(true)
    })

    // The binding should now use the new name
    const win = result.current.workspaces.terminal1.windows[0]
    expect(win.boundSessions).toContain('new-name')
    expect(win.boundSessions).not.toContain('old-name')
    expect(win.activeSession).toBe('new-name')
  })

  it('updates activeSession when it matches the old name', async () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'rename-me')
    })
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'keep-me')
    })

    // Set rename-me as active
    act(() => {
      result.current.setActiveSession('terminal1', 'terminal1-window-0', 'rename-me')
    })

    await act(async () => {
      await result.current.renameSession('rename-me', 'renamed')
    })

    const win = result.current.workspaces.terminal1.windows[0]
    expect(win.activeSession).toBe('renamed')
    expect(win.boundSessions).toContain('renamed')
    expect(win.boundSessions).toContain('keep-me')
  })

  it('globally keeps the first canonical rename claimant and repairs later visible collisions', async () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'first-fallback')
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'old', 'alice')
      result.current.addSessionToWindow('terminal2', 'terminal2-window-1', 'later-fallback')
      result.current.addSessionToWindow('terminal2', 'terminal2-window-1', 'new', 'alice')
      result.current.addSessionToWindow('terminal3', 'terminal3-window-0', 'new', 'bob')
      result.current.addSessionToWindow('terminal3', 'terminal3-window-1', 'new')
    })

    await act(async () => {
      expect(await result.current.renameSession('old', 'new', 'alice')).toBe(true)
    })

    expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
      boundSessions: ['first-fallback', 'alice:new'],
      activeSession: 'alice:new',
    })
    expect(result.current.workspaces.terminal2.windows[1]).toMatchObject({
      boundSessions: ['later-fallback'],
      activeSession: 'later-fallback',
    })
    expect(result.current.workspaces.terminal3.windows[0].boundSessions).toEqual(['bob:new'])
    expect(result.current.workspaces.terminal3.windows[1].boundSessions).toEqual(['new'])

    expect(result.current.assignedSessions.get('alice:new')).toMatchObject({
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-0',
    })
    expect([...result.current.assignedSessions.keys()].filter(key => key === 'alice:new')).toHaveLength(1)

    const visibleTerminalWindowCandidates = Object.values(result.current.workspaces)
      .flatMap(workspace => workspace.windows.slice(0, workspace.windowCount))
      .filter(window => window.boundSessions.includes('alice:new'))
    expect(visibleTerminalWindowCandidates).toHaveLength(1)
  })

  it('renames only the qualified user when the old and new bare aliases are ambiguous', async () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'old', 'alice')
      result.current.addSessionToWindow('terminal1', 'terminal1-window-1', 'old', 'bob')
      result.current.addSessionToWindow('terminal2', 'terminal2-window-0', 'old')
      result.current.addSessionToWindow('terminal2', 'terminal2-window-1', 'new-fallback')
      result.current.addSessionToWindow('terminal2', 'terminal2-window-1', 'new', 'alice')
      result.current.addSessionToWindow('terminal3', 'terminal3-window-0', 'new', 'bob')
      result.current.addSessionToWindow('terminal3', 'terminal3-window-1', 'new')
    })

    await act(async () => {
      expect(await result.current.renameSession('old', 'new', 'alice')).toBe(true)
    })

    expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
      boundSessions: ['alice:new'],
      activeSession: 'alice:new',
    })
    expect(result.current.workspaces.terminal1.windows[1]).toMatchObject({
      boundSessions: ['bob:old'],
      activeSession: 'bob:old',
    })
    expect(result.current.workspaces.terminal2.windows[0]).toMatchObject({
      boundSessions: ['old'],
      activeSession: 'old',
    })
    expect(result.current.workspaces.terminal2.windows[1]).toMatchObject({
      boundSessions: ['new-fallback'],
      activeSession: 'new-fallback',
    })
    expect(result.current.workspaces.terminal3.windows[0].boundSessions).toEqual(['bob:new'])
    expect(result.current.workspaces.terminal3.windows[1].boundSessions).toEqual(['new'])

    const bindings = Object.values(result.current.workspaces)
      .flatMap(workspace => workspace.windows)
      .flatMap(window => window.boundSessions)
    expect(bindings.filter(binding => binding === 'alice:new')).toHaveLength(1)
    expect(bindings).toEqual(expect.arrayContaining(['bob:old', 'old', 'bob:new', 'new']))
  })

  it('keeps a bare rename exact when qualified same-name sessions also exist', async () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'old', 'alice')
      result.current.addSessionToWindow('terminal1', 'terminal1-window-1', 'old', 'bob')
      result.current.addSessionToWindow('terminal2', 'terminal2-window-0', 'old')
    })

    await act(async () => {
      expect(await result.current.renameSession('old', 'new')).toBe(true)
    })

    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:old'])
    expect(result.current.workspaces.terminal1.windows[1].boundSessions).toEqual(['bob:old'])
    expect(result.current.workspaces.terminal2.windows[0]).toMatchObject({
      boundSessions: ['new'],
      activeSession: 'new',
    })
  })

  it('returns false when API call fails', async () => {
    const { result } = renderSession()

    // Queue a failing response for the next fetch call (the rename PATCH); the
    // default mount refresh remains pending in this fixture.
    vi.mocked(fetch as any).mockResolvedValueOnce({
      ok: false,
      text: () => Promise.resolve('Not Found'),
      json: () => Promise.resolve({}),
    })

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'wont-rename')
    })

    let success: boolean | undefined
    await act(async () => {
      success = await result.current.renameSession('wont-rename', 'new-name')
    })

    expect(success).toBe(false)
    // Binding should remain unchanged
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toContain('wont-rename')
  })
})


// ──────────────────────────────────────────────
// 6. canonical slots, reveal contract, viewport persistence, and preset identity
// ──────────────────────────────────────────────
describe('canonical terminal layout invariants', () => {
  beforeEach(() => {
    localStorage.clear()
    setViewportWidth(1280)
  })

  it('stores four canonical slots in every default workspace while windowCount remains visibility only', () => {
    const { result } = renderSession()

    ;(['terminal1', 'terminal2', 'terminal3'] as const).forEach(workspaceId => {
      expect(result.current.workspaces[workspaceId].windowCount).toBe(2)
      expect(result.current.workspaces[workspaceId].windows.map(window => window.id)).toEqual([
        `${workspaceId}-window-0`,
        `${workspaceId}-window-1`,
        `${workspaceId}-window-2`,
        `${workspaceId}-window-3`,
      ])
    })
  })

  it('preserves hidden bindings, active choice, and assignedSessions through shrink, re-expand, and reload', () => {
    const initial = renderSession()

    act(() => {
      initial.result.current.setWindowCount('terminal1', 4)
      initial.result.current.addSessionToWindow('terminal1', 'terminal1-window-3', 'hidden-a', 'alice')
      initial.result.current.addSessionToWindow('terminal1', 'terminal1-window-3', 'hidden-b', 'alice')
      initial.result.current.setActiveSession('terminal1', 'terminal1-window-3', 'alice:hidden-a')
      initial.result.current.setWindowCount('terminal1', 1)
    })

    expect(initial.result.current.workspaces.terminal1.windows).toHaveLength(4)
    expect(initial.result.current.workspaces.terminal1.windows[3]).toMatchObject({
      boundSessions: ['alice:hidden-a', 'alice:hidden-b'],
      activeSession: 'alice:hidden-a',
    })
    expect(initial.result.current.assignedSessions.get('alice:hidden-a')).toMatchObject({
      windowId: 'terminal1-window-3',
      windowIndex: 4,
    })

    act(() => initial.result.current.setWindowCount('terminal1', 4))
    expect(initial.result.current.workspaces.terminal1.windows[3].activeSession).toBe('alice:hidden-a')
    act(() => initial.result.current.setWindowCount('terminal1', 1))
    initial.unmount()

    const reloaded = renderSession()
    expect(reloaded.result.current.workspaces.terminal1.windowCount).toBe(1)
    expect(reloaded.result.current.workspaces.terminal1.windows[3]).toMatchObject({
      boundSessions: ['alice:hidden-a', 'alice:hidden-b'],
      activeSession: 'alice:hidden-a',
    })
  })

  it('persists all four slots independently in mobile, tablet, and desktop viewport buckets', () => {
    const buckets = [
      { width: 390, session: 'mobile-hidden' },
      { width: 900, session: 'tablet-hidden' },
      { width: 1280, session: 'desktop-hidden' },
    ]

    buckets.forEach(({ width, session }) => {
      setViewportWidth(width)
      const mounted = renderSession()
      act(() => {
        mounted.result.current.addSessionToWindow('terminal2', 'terminal2-window-3', session)
        mounted.result.current.setWindowCount('terminal2', 1)
      })
      mounted.unmount()
    })

    buckets.forEach(({ width, session }) => {
      setViewportWidth(width)
      const reloaded = renderSession()
      expect(reloaded.result.current.workspaces.terminal2.windows).toHaveLength(4)
      expect(reloaded.result.current.workspaces.terminal2.windows[3].boundSessions).toEqual([session])
      expect(reloaded.result.current.workspaces.terminal2.windowCount).toBe(1)
      reloaded.unmount()
    })
  })

  it('reveals a hidden canonical slot before the navigation slice focuses it', () => {
    const { result } = renderSession()

    act(() => result.current.setWindowCount('terminal3', 1))
    act(() => result.current.revealWindow('terminal3', 'terminal3-window-2'))

    expect(result.current.workspaces.terminal3.windowCount).toBe(3)
    expect(result.current.windowRevealRequest).toMatchObject({
      workspaceId: 'terminal3',
      windowId: 'terminal3-window-2',
    })
    expect(result.current.focusedWindowKey).toBeNull()

    act(() => result.current.setFocusedWindowKey('terminal3-terminal3-window-2'))
    expect(result.current.focusedWindowKey).toBe('terminal3-terminal3-window-2')
  })

  it('leaves visibility and reveal state unchanged for malformed or noncanonical targets', () => {
    const { result } = renderSession()

    act(() => result.current.setWindowCount('terminal3', 1))
    expect(result.current.windowRevealRequest).toBeNull()

    act(() => result.current.revealWindow('terminal3', 'terminal3-window-03'))
    expect(result.current.workspaces.terminal3.windowCount).toBe(1)
    expect(result.current.windowRevealRequest).toBeNull()

    act(() => result.current.revealWindow('missing-workspace' as never, 'missing-workspace-window-0'))
    expect(result.current.workspaces.terminal3.windowCount).toBe(1)
    expect(result.current.windowRevealRequest).toBeNull()
  })

  it('issues a fresh increasing request ID for every repeated valid reveal', () => {
    const { result } = renderSession()

    act(() => result.current.revealWindow('terminal2', 'terminal2-window-1'))
    const firstRequest = result.current.windowRevealRequest
    act(() => result.current.revealWindow('terminal2', 'terminal2-window-1'))
    const secondRequest = result.current.windowRevealRequest

    expect(firstRequest).toMatchObject({ workspaceId: 'terminal2', windowId: 'terminal2-window-1' })
    expect(secondRequest).toMatchObject({ workspaceId: 'terminal2', windowId: 'terminal2-window-1' })
    expect(secondRequest!.requestId).toBeGreaterThan(firstRequest!.requestId)
  })

  it('normalizes stored presets and applied layouts to one claimable slot per safe identity', () => {
    localStorage.setItem('chrote-dashboard-presets', JSON.stringify([{
      id: 'historical',
      name: 'Historical duplicates',
      createdAt: 1,
      workspaces: {
        terminal1: {
          windowCount: 2,
          windows: [
            { id: 'old-0', boundSessions: ['alice:solo', 'alice:shared'], activeSession: 'alice:solo', colorIndex: 0 },
            { id: 'old-1', boundSessions: ['solo', 'fallback'], activeSession: 'solo', colorIndex: 1 },
          ],
        },
        terminal2: {
          windowCount: 2,
          windows: [
            { id: 'old-2', boundSessions: ['bob:shared', 'shared'], activeSession: 'shared', colorIndex: 2 },
            { id: 'old-3', boundSessions: ['alice:solo', 'keep'], activeSession: 'alice:solo', colorIndex: 3 },
          ],
        },
      },
    }]))

    const { result } = renderSession()
    const preset = result.current.layoutPresets[0]

    expect(preset.workspaces.terminal1.windows).toHaveLength(4)
    expect(preset.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:solo', 'alice:shared'])
    expect(preset.workspaces.terminal1.windows[1]).toMatchObject({
      boundSessions: ['fallback'],
      activeSession: 'fallback',
    })
    expect(preset.workspaces.terminal2.windows[0].boundSessions).toEqual(['bob:shared', 'shared'])
    expect(preset.workspaces.terminal2.windows[1]).toMatchObject({
      boundSessions: ['keep'],
      activeSession: 'keep',
    })

    act(() => result.current.loadPreset('historical'))
    const claimableSlots = Object.values(result.current.workspaces)
      .flatMap(workspace => workspace.windows)
      .filter(window => window.boundSessions.includes('alice:solo') || window.boundSessions.includes('solo'))
    expect(claimableSlots).toHaveLength(1)
    expect(result.current.assignedSessions.get('alice:solo')).toMatchObject({
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-0',
    })
  })

  it('keeps the stable first safe identity, exact-dedupes, preserves ambiguous users, and repairs only invalid active choices', () => {
    localStorage.setItem('chrote-dashboard-presets', JSON.stringify([{
      id: 'identity-audit',
      name: 'Identity audit',
      createdAt: 2,
      workspaces: {
        terminal1: {
          windowCount: 4,
          windows: [
            {
              id: 'legacy-0',
              boundSessions: ['solo', 'solo', 'alice:solo', 'keep-active'],
              activeSession: 'keep-active',
              colorIndex: 0,
            },
            {
              id: 'legacy-1',
              boundSessions: ['alice:solo', 'fallback'],
              activeSession: 'alice:solo',
              colorIndex: 1,
            },
            {
              id: 'legacy-2',
              boundSessions: ['alice:shared', 'bob:shared', 'shared', 'shared'],
              activeSession: 'bob:shared',
              colorIndex: 2,
            },
            {
              id: 'legacy-3',
              boundSessions: ['first', 'second'],
              activeSession: 'missing',
              colorIndex: 3,
            },
            {
              id: 'legacy-4',
              boundSessions: ['must-be-dropped'],
              activeSession: 'must-be-dropped',
              colorIndex: 4,
            },
          ],
        },
      },
    }]))

    const { result } = renderSession()
    const windows = result.current.layoutPresets[0].workspaces.terminal1.windows

    expect(windows).toHaveLength(4)
    expect(windows[0]).toMatchObject({
      id: 'terminal1-window-0',
      boundSessions: ['solo', 'keep-active'],
      activeSession: 'keep-active',
    })
    expect(windows[1]).toMatchObject({ boundSessions: ['fallback'], activeSession: 'fallback' })
    expect(windows[2]).toMatchObject({
      boundSessions: ['alice:shared', 'bob:shared', 'shared'],
      activeSession: 'bob:shared',
    })
    expect(windows[3]).toMatchObject({ boundSessions: ['first', 'second'], activeSession: 'first' })
    expect(windows.flatMap(window => window.boundSessions)).not.toContain('must-be-dropped')
  })

  it('sanitizes and deduplicates the save-current-layout path', async () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'old')
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'existing')
    })
    await act(async () => {
      expect(await result.current.renameSession('old', 'existing')).toBe(true)
    })
    expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
      boundSessions: ['existing'],
      activeSession: 'existing',
    })

    const liveClaims = Object.values(result.current.workspaces)
      .flatMap(workspace => workspace.windows)
      .filter(window => window.boundSessions.includes('existing'))
    expect(liveClaims).toHaveLength(1)

    act(() => expect(result.current.saveCurrentLayout('sanitized')).toBe(true))
    const saved = result.current.layoutPresets.find(preset => preset.name === 'sanitized')
    expect(saved?.workspaces.terminal1.windows).toHaveLength(4)
    expect(saved?.workspaces.terminal1.windows[0]).toMatchObject({
      boundSessions: ['existing'],
      activeSession: 'existing',
    })
    const savedClaims = Object.values(saved!.workspaces)
      .flatMap(workspace => workspace.windows)
      .filter(window => window.boundSessions.includes('existing'))
    expect(savedClaims).toHaveLength(1)
  })

  it('opens Peek for an assigned hidden session row without navigating or mutating its assignment', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal2', 'terminal2-window-3', 'hidden', 'alice')
      result.current.setWindowCount('terminal2', 1)
    })
    const before = result.current.workspaces.terminal2

    act(() => {
      result.current.handleSessionClick('alice:hidden')
    })

    expect(result.current.floatingSession).toBe('alice:hidden')
    expect(result.current.workspaces.terminal2).toEqual(before)
    expect(result.current.focusedWindowKey).toBeNull()
    expect(result.current.windowRevealRequest).toBeNull()
  })

  it('reveals, activates, and focuses an assigned hidden session through its explicit location action', () => {
    const { result } = renderSession()

    act(() => {
      result.current.addSessionToWindow('terminal2', 'terminal2-window-3', 'hidden', 'alice')
      result.current.setWindowCount('terminal2', 1)
    })
    act(() => {
      result.current.focusSessionAssignment('alice:hidden')
    })

    expect(result.current.workspaces.terminal2.windowCount).toBe(4)
    expect(result.current.workspaces.terminal2.windows[3].activeSession).toBe('alice:hidden')
    expect(result.current.focusedWindowKey).toBe('terminal2-terminal2-window-3')
    expect(result.current.windowRevealRequest).toMatchObject({
      workspaceId: 'terminal2',
      windowId: 'terminal2-window-3',
    })
    expect(result.current.floatingSession).toBeNull()
  })
})

// ──────────────────────────────────────────────
// 7. clampWindowCount — boundary values
// ──────────────────────────────────────────────
describe('clampWindowCount (via setWindowCount)', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('clamps 0 to 1', () => {
    const { result } = renderSession()

    act(() => {
      result.current.setWindowCount('terminal1', 0)
    })

    expect(result.current.workspaces.terminal1.windowCount).toBe(1)
    expect(result.current.workspaces.terminal1.windows).toHaveLength(4)
  })

  it('clamps 5 to 4', () => {
    const { result } = renderSession()

    act(() => {
      result.current.setWindowCount('terminal1', 5)
    })

    expect(result.current.workspaces.terminal1.windowCount).toBe(4)
    expect(result.current.workspaces.terminal1.windows).toHaveLength(4)
  })

  it('keeps 1 as 1', () => {
    const { result } = renderSession()

    act(() => {
      result.current.setWindowCount('terminal1', 1)
    })

    expect(result.current.workspaces.terminal1.windowCount).toBe(1)
    expect(result.current.workspaces.terminal1.windows).toHaveLength(4)
  })

  it('keeps 4 as 4', () => {
    const { result } = renderSession()

    act(() => {
      result.current.setWindowCount('terminal1', 4)
    })

    expect(result.current.workspaces.terminal1.windowCount).toBe(4)
    expect(result.current.workspaces.terminal1.windows).toHaveLength(4)
  })

  it('clamps negative numbers to 1', () => {
    const { result } = renderSession()

    act(() => {
      result.current.setWindowCount('terminal1', -3)
    })

    expect(result.current.workspaces.terminal1.windowCount).toBe(1)
  })

  it('preserves existing window data when increasing count', () => {
    const { result } = renderSession()

    // Start with 2 windows, bind a session
    act(() => {
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'persist-me')
    })

    // Increase to 4
    act(() => {
      result.current.setWindowCount('terminal1', 4)
    })

    expect(result.current.workspaces.terminal1.windows).toHaveLength(4)
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toContain('persist-me')
    // New windows should be empty
    expect(result.current.workspaces.terminal1.windows[2].boundSessions).toEqual([])
    expect(result.current.workspaces.terminal1.windows[3].boundSessions).toEqual([])
  })
})

// ──────────────────────────────────────────────
describe('terminal tab count', () => {
  beforeEach(() => {
    localStorage.clear()
    setViewportWidth(1280)
  })

  it('defaults to three visible workspaces', () => {
    const { result } = renderSession()
    expect(result.current.settings.terminalTabCount).toBe(3)
    expect(result.current.workspaceIds).toEqual(['terminal1', 'terminal2', 'terminal3'])
  })

  it('normalizes malformed counts through updateSettings', () => {
    const { result } = renderSession()

    act(() => result.current.updateSettings({ terminalTabCount: 9 }))
    expect(result.current.settings.terminalTabCount).toBe(6)

    act(() => result.current.updateSettings({ terminalTabCount: 0 }))
    expect(result.current.settings.terminalTabCount).toBe(1)

    act(() => result.current.updateSettings({ terminalTabCount: 2.7 }))
    expect(result.current.settings.terminalTabCount).toBe(2)

    act(() => result.current.updateSettings({ terminalTabCount: Number.NaN }))
    expect(result.current.settings.terminalTabCount).toBe(3)
  })

  it('normalizes malformed stored counts at load time', () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {},
      sidebarCollapsed: false,
      settings: { ...DEFAULT_SETTINGS, terminalTabCount: '5' },
    }))

    const { result } = renderSession()
    expect(result.current.settings.terminalTabCount).toBe(3)
  })

  it('grows by revealing default workspaces and keeps them across the visible list', () => {
    const { result } = renderSession()

    act(() => result.current.updateSettings({ terminalTabCount: 5 }))

    expect(result.current.workspaceIds).toEqual(['terminal1', 'terminal2', 'terminal3', 'terminal4', 'terminal5'])
    expect(result.current.workspaces.terminal4.windows).toHaveLength(4)
    expect(result.current.workspaces.terminal5.windowCount).toBe(2)
  })

  it('shrink hides tabs but preserves the workspace record, then grow restores it exactly', () => {
    const { result } = renderSession()

    // Non-default terminal3 state: window count, binding, label, launch user.
    act(() => result.current.setWindowCount('terminal3', 3))
    act(() => result.current.addSessionToWindow('terminal3', 'terminal3-window-1', 'ops-shell', 'build'))
    act(() => result.current.updateSettings({
      terminalLabels: { terminal3: 'Ops' },
      terminalLaunchUsers: { terminal3: 'build' },
    }))

    const before = JSON.parse(JSON.stringify(result.current.workspaces.terminal3))

    act(() => result.current.updateSettings({ terminalTabCount: 2 }))
    expect(result.current.workspaceIds).toEqual(['terminal1', 'terminal2'])
    expect(result.current.workspaces.terminal3).toEqual(before)
    expect(result.current.settings.terminalLabels.terminal3).toBe('Ops')
    expect(result.current.settings.terminalLaunchUsers.terminal3).toBe('build')

    act(() => result.current.updateSettings({ terminalTabCount: 3 }))
    expect(result.current.workspaceIds).toEqual(['terminal1', 'terminal2', 'terminal3'])
    expect(result.current.workspaces.terminal3).toEqual(before)
  })

  it('round-trips a hidden workspace through storage across a reload', () => {
    const first = renderSession()

    act(() => first.result.current.setWindowCount('terminal3', 3))
    act(() => first.result.current.addSessionToWindow('terminal3', 'terminal3-window-1', 'ops-shell', 'build'))
    act(() => first.result.current.updateSettings({ terminalTabCount: 2 }))
    const before = JSON.parse(JSON.stringify(first.result.current.workspaces.terminal3))
    first.unmount()

    const second = renderSession()
    expect(second.result.current.settings.terminalTabCount).toBe(2)
    expect(second.result.current.workspaceIds).toEqual(['terminal1', 'terminal2'])
    expect(second.result.current.workspaces.terminal3).toEqual(before)

    act(() => second.result.current.updateSettings({ terminalTabCount: 3 }))
    expect(second.result.current.workspaces.terminal3).toEqual(before)
  })

  it('keeps the version-sensitive theme reset for old stored settings and defaults their count', () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 1,
      layoutsByViewport: {},
      sidebarCollapsed: false,
      settings: { ...DEFAULT_SETTINGS, theme: 'matrix', terminalTabCount: undefined },
    }))

    const { result } = renderSession()
    expect(result.current.settings.theme).toBe('dark')
    expect(result.current.settings.tmuxAppearance).toEqual(DEFAULT_TMUX_APPEARANCE)
    expect(result.current.settings.terminalTabCount).toBe(3)
  })

  it('retains launch users stored for hidden workspace ids', () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {},
      sidebarCollapsed: false,
      settings: { ...DEFAULT_SETTINGS, terminalTabCount: 2, terminalLaunchUsers: { terminal5: 'build' } },
    }))

    const { result } = renderSession()
    expect(result.current.settings.terminalLaunchUsers.terminal5).toBe('build')
  })

  it('loading a smaller preset preserves current workspaces absent from it and never changes the count', () => {
    localStorage.setItem('chrote-dashboard-presets', JSON.stringify([{
      id: 'preset-small',
      name: 'Solo',
      createdAt: 1,
      workspaces: {
        terminal1: {
          windowCount: 1,
          windows: [{ id: 'terminal1-window-0', boundSessions: ['alice:solo'], activeSession: 'alice:solo', colorIndex: 0 }],
        },
      },
    }]))

    const { result } = renderSession()
    act(() => result.current.setWindowCount('terminal3', 4))
    const terminal3Before = JSON.parse(JSON.stringify(result.current.workspaces.terminal3))

    act(() => result.current.loadPreset('preset-small'))

    expect(result.current.settings.terminalTabCount).toBe(3)
    expect(result.current.workspaceIds).toHaveLength(3)
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:solo'])
    expect(result.current.workspaces.terminal3).toEqual(terminal3Before)
  })

  it('loading a larger preset stores its extra workspaces hidden without changing the count', () => {
    localStorage.setItem('chrote-dashboard-presets', JSON.stringify([{
      id: 'preset-large',
      name: 'Fleet',
      createdAt: 1,
      workspaces: {
        terminal1: { windowCount: 1, windows: [] },
        terminal2: { windowCount: 1, windows: [] },
        terminal3: { windowCount: 1, windows: [] },
        terminal4: {
          windowCount: 1,
          windows: [{ id: 'terminal4-window-0', boundSessions: ['alice:extra'], activeSession: 'alice:extra', colorIndex: 0 }],
        },
      },
    }]))

    const { result } = renderSession()
    act(() => result.current.loadPreset('preset-large'))

    expect(result.current.settings.terminalTabCount).toBe(3)
    expect(result.current.workspaceIds).toEqual(['terminal1', 'terminal2', 'terminal3'])
    expect(result.current.workspaces.terminal4.windows[0].boundSessions).toEqual(['alice:extra'])

    act(() => result.current.updateSettings({ terminalTabCount: 4 }))
    expect(result.current.workspaceIds).toContain('terminal4')
    expect(result.current.workspaces.terminal4.windows[0].boundSessions).toEqual(['alice:extra'])
  })
})
