import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, waitFor } from '@testing-library/react'
import { featureFlagKey } from '../featureFlags'
import { DEFAULT_SETTINGS } from '../types'
import {
  renderSession,
  setViewportWidth,
  store,
  storedDashboardState,
} from './SessionContext.test.support'

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
      },
    }))

    const { result } = renderSession()

    expect(result.current.settings).toEqual({
      ...DEFAULT_SETTINGS,
      fontSize: 17,
    })
  })

  it('does not mutate tmux on initial load and applies mouse mode only after an explicit setting change', async () => {
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

    await waitFor(() => expect(result.current.settings.mouseScroll).toBe(false))
    expect(hasMouseCall(false)).toBe(false)

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

  it('migrates a stored INIT-PENDING binding to no active session', async () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [
                { id: 'terminal1-window-0', boundSessions: ['pending', 'INIT-PENDING'], activeSession: 'INIT-PENDING', colorIndex: 0 },
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
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['pending'])

    const persisted = storedDashboardState()
    expect(persisted.layoutsByViewport.desktop.workspaces.terminal1.windows[0]).toEqual({
      id: 'terminal1-window-0',
      boundSessions: ['pending'],
      activeSession: null,
      colorIndex: 0,
    })
  })
})

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
    expect(result.current.settings).toEqual(DEFAULT_SETTINGS)
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

  // The host owns the theme now, and the tmux palette, the per-user badge
  // colours and the music went with it. A browser that still holds those keys
  // is the normal case after this ships, so loading has to read past them.
  it('loads settings saved before the theme moved to the host, ignoring the retired keys', () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 1,
      layoutsByViewport: {},
      sidebarCollapsed: false,
      settings: {
        ...DEFAULT_SETTINGS,
        theme: 'matrix',
        tmuxAppearance: { statusFg: '#00ff41' },
        terminalUserColors: { alice: '#123456' },
        musicVolume: 0.8,
        musicEnabled: true,
        fontSize: 18,
        terminalTabCount: undefined,
      },
    }))

    const { result } = renderSession()
    expect(result.current.settings).toEqual({ ...DEFAULT_SETTINGS, fontSize: 18 })
    expect(result.current.settings.terminalTabCount).toBe(3)
  })

  it('persists appearance and polling values through the shared settings update', () => {
    const { result } = renderSession()

    act(() => result.current.updateSettings({
      fontSize: 18,
      autoRefreshInterval: 10000,
    }))

    expect(result.current.settings).toMatchObject({
      fontSize: 18,
      autoRefreshInterval: 10000,
    })
    expect(JSON.parse(localStorage.getItem('chrote-dashboard-state') || '{}').settings).toMatchObject({
      fontSize: 18,
      autoRefreshInterval: 10000,
    })
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
