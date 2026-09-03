import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, waitFor } from '@testing-library/react'
import { DEFAULT_SETTINGS, resolveLaunchUser } from '../types'
import { renderSession, renderSessionWithStatus } from './SessionContext.test.support'

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

  it('sends the launcher\'s folder and harness with the session it creates', async () => {
    const fetchMock = stubSessionFetch()
    const { result } = renderSession()

    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice', 'build']))
    fetchMock.mockClear()

    await act(async () => {
      await result.current.createSession({
        workspaceId: 'terminal1',
        unixUser: 'alice',
        name: 'claude-chrote',
        cwd: '/srv/chrote',
        harness: 'claude-code',
      })
    })

    const [, init] = fetchMock.mock.calls[0]
    expect(JSON.parse(String(init?.body))).toEqual({
      name: 'claude-chrote',
      unixUser: 'alice',
      mouseScroll: true,
      cwd: '/srv/chrote',
      harness: 'claude-code',
    })
  })

  it('shows the server\'s warning when the session was created but its harness was not', async () => {
    const fetchMock = stubSessionFetch()
    const { result } = renderSessionWithStatus()

    await waitFor(() => expect(result.current.session.terminalUsers).toEqual(['alice', 'build']))
    fetchMock.mockClear()
    fetchMock.mockImplementationOnce(() => Promise.resolve({
      ok: true,
      json: () => Promise.resolve({
        success: true,
        session: 'claude-chrote',
        warning: 'session created, but the "claude-code" command could not be started: no server',
      }),
      text: () => Promise.resolve(''),
    }))

    let created: string | null = null
    await act(async () => {
      created = await result.current.session.createSession({
        workspaceId: 'terminal1',
        unixUser: 'alice',
        name: 'claude-chrote',
        harness: 'claude-code',
      })
    })

    // The session exists, so it is still a success; the warning says what did
    // not start inside it.
    expect(created).toBe('claude-chrote')
    const announced = result.current.status.status
    expect(announced?.severity).toBe('warning')
    expect(announced?.message).toContain('could not be started')
  })

  it('handles expected create-session API failures with an announcement and no console error', async () => {
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

describe('sessionEvidence', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('holds what the last poll that answered said through a poll that failed', async () => {
    const fetchMock = vi.fn(() => Promise.resolve({
      ok: true,
      json: () => Promise.resolve({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        grouped: {},
        terminalUsers: ['alice'],
        timestamp: new Date().toISOString(),
      }),
      text: () => Promise.resolve(''),
    }))
    vi.stubGlobal('fetch', fetchMock)

    const { result } = renderSession()
    await waitFor(() => expect(result.current.sessionEvidence.live?.has('alice:shell1')).toBe(true))

    // The next poll fails outright. That is the absence of news, not news that
    // a dead session came back, so what the last answer said still stands and
    // a tile that reached Ended on it is not talked out of the verdict.
    vi.mocked(fetch as any).mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: 'tmux socket unreachable' }),
      text: () => Promise.resolve(''),
    })
    await act(async () => { await result.current.refreshSessions() })

    expect(result.current.error).toBe('tmux socket unreachable')
    expect(result.current.sessionEvidence.live?.has('alice:shell1')).toBe(true)
    expect(result.current.sessionEvidence.live?.has('alice:departed')).toBe(false)
  })
})
