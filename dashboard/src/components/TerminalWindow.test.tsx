import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalWindow from './TerminalWindow'
import { DEFAULT_SETTINGS } from '../types'

const refreshSessions = vi.fn()
const addSessionToWindow = vi.fn()
const addToast = vi.fn()
const removeSessionFromWindow = vi.fn()
const renameSession = vi.fn()
const deleteSession = vi.fn()
const reconnectIframe = vi.fn()
const setFocusedWindowKey = vi.fn()

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: {
      ...DEFAULT_SETTINGS,
      terminalLaunchUsers: {
        ...DEFAULT_SETTINGS.terminalLaunchUsers,
        terminal3: 'tavern',
      },
      terminalSessionPrefixes: {
        ...DEFAULT_SETTINGS.terminalSessionPrefixes,
        tavern: 'forge',
      },
    },
    terminalUsers: ['perttu', 'tavern'],
    sessions: [
      { name: 'forge-existing', windows: 1, attached: false, group: 'forge', unixUser: 'tavern' },
      { name: 'shell-existing', windows: 1, attached: false, group: 'shell', unixUser: 'perttu' },
    ],
    layoutPresets: [{ id: 'preset-1', name: 'Focus Layout', createdAt: 1, workspaces: {} }],
    refreshSessions,
    addSessionToWindow,
    removeSessionFromWindow,
    setActiveSession: vi.fn(),
    cycleSession: vi.fn(),
    setWindowCount: vi.fn(),
    clearStaleSessionsFromWindow: vi.fn(),
    focusedWindowKey: null,
    setFocusedWindowKey,
    saveCurrentLayout: vi.fn(),
    loadPreset: vi.fn(),
    deleteSession,
    renameSession,
  }),
}))

vi.mock('../context/ToastContext', () => ({
  useToast: () => ({ addToast }),
}))

vi.mock('./IframePool', () => ({
  useIframePool: () => ({
    claimIframe: vi.fn(() => vi.fn()),
    isLoaded: vi.fn(() => false),
    getIframe: vi.fn(() => null),
    triggerFit: vi.fn(),
    focusIframe: vi.fn(),
    reconnectIframe,
  }),
}))

describe('TerminalWindow launch user', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    deleteSession.mockResolvedValue(true)
    renameSession.mockResolvedValue(true)
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({ ok: true })) as any)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
  })

  it('creates new sessions with the workspace configured Unix user', async () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: /New Session/i }))

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled())
    const [, init] = (globalThis.fetch as any).mock.calls[0]
    expect(JSON.parse(init.body)).toMatchObject({ unixUser: 'tavern' })
    expect(JSON.parse(init.body).name).toMatch(/^forge-/)
  })

  it('binds the new session immediately instead of waiting for aggregate refresh', async () => {
    refreshSessions.mockReturnValue(new Promise(() => {}))

    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: /New Session/i }))

    await waitFor(() => expect(addSessionToWindow).toHaveBeenCalled())
    expect(refreshSessions).toHaveBeenCalled()
    expect(addSessionToWindow.mock.invocationCallOrder[0]).toBeLessThan(refreshSessions.mock.invocationCallOrder[0])
    const addedSession = addSessionToWindow.mock.calls[0][2]
    expect(addedSession).toMatch(/^forge-/)
  })

  it('keeps session tag context menu limited to rename and kill', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.contextMenu(screen.getByText('forge-existing'))

    expect(screen.getByRole('button', { name: /Rename/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Kill/i })).toBeInTheDocument()
    expect(screen.queryByText(/Move/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Detach/i)).not.toBeInTheDocument()
  })

  it('does not focus or refit the terminal iframe when clicking the session tab bar', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(container.querySelector('.terminal-window-header')!)

    expect(setFocusedWindowKey).not.toHaveBeenCalled()

    fireEvent.click(container.querySelector('.terminal-window-body')!)
    expect(setFocusedWindowKey).toHaveBeenCalledWith('terminal3-terminal3-window-0')
  })
})
