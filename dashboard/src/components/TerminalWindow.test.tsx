import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalWindow from './TerminalWindow'
import { DEFAULT_SETTINGS } from '../types'

const refreshSessions = vi.fn()
const createSession = vi.fn()
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
    createSession,
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
    createSession.mockResolvedValue('forge1')
    deleteSession.mockResolvedValue(true)
    renameSession.mockResolvedValue(true)
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({ ok: true })) as any)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
  })

  it('creates new sessions with the shared creation action and attaches to the current window', async () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: /New Session/i }))

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({
      workspaceId: 'terminal3',
      attachTo: { workspaceId: 'terminal3', windowId: 'terminal3-window-0' },
    })
  })

  it('passes explicit user choices from the window new-session menu into the same shared action', async () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    fireEvent.contextMenu(screen.getByRole('button', { name: /New Session/i }))
    fireEvent.click(screen.getByRole('button', { name: /New here as P perttu/i }))

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({
      workspaceId: 'terminal3',
      unixUser: 'perttu',
      attachTo: { workspaceId: 'terminal3', windowId: 'terminal3-window-0' },
    })
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

  it('does not focus from generic header clicks but focuses when clicking the status dot or body', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(container.querySelector('.terminal-window-header')!)

    expect(setFocusedWindowKey).not.toHaveBeenCalled()

    fireEvent.click(container.querySelector('.status-dot')!)
    expect(setFocusedWindowKey).toHaveBeenCalledWith('terminal3-terminal3-window-0')

    setFocusedWindowKey.mockClear()

    fireEvent.click(container.querySelector('.terminal-window-body')!)
    expect(setFocusedWindowKey).toHaveBeenCalledWith('terminal3-terminal3-window-0')
  })
})
