import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SessionPanel from './SessionPanel'
import { DEFAULT_SETTINGS } from '../types'

const refreshSessions = vi.fn()
const createSession = vi.fn()
const addToast = vi.fn()
const fetchMock = vi.fn()

const mockState = vi.hoisted(() => ({
  sessions: [] as Array<Record<string, unknown>>,
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    groupedSessions: {},
    loading: false,
    error: null,
    sidebarCollapsed: false,
    toggleSidebar: vi.fn(),
    refreshSessions,
    createSession,
    sessions: mockState.sessions,
    settings: {
      ...DEFAULT_SETTINGS,
      terminalSessionPrefixes: { alice: 'alice', bob: 'bob' },
    },
    terminalUsers: ['alice', 'bob'],
  }),
}))

vi.mock('../context/ToastContext', () => ({
  useToast: () => ({ addToast }),
}))

vi.mock('./NukeConfirmModal', () => ({
  default: () => null,
}))

describe('SessionPanel new-session context menu', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockState.sessions.length = 0
    createSession.mockResolvedValue('created')
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ success: true, removed: true }) })
    vi.stubGlobal('fetch', fetchMock)
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('creates a default side-panel session in the visible terminal workspace', async () => {
    render(<SessionPanel activeWorkspaceId="terminal3" />)

    fireEvent.click(screen.getByTitle('New tmux session'))

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({ workspaceId: 'terminal3' })
  })

  it('creates a new session as the selected configured Unix user from the New Session context menu', async () => {
    render(<SessionPanel activeWorkspaceId="terminal2" />)

    fireEvent.click(screen.getByRole('button', { name: 'Session creation options' }))
    fireEvent.click(screen.getByRole('button', { name: /New as B bob/i }))
    expect(screen.queryByRole('button', { name: /New as B bob/i })).not.toBeInTheDocument()

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({ workspaceId: 'terminal2', unixUser: 'bob' })
  })

  it('opens a named session field and creates the exact typed session name', async () => {
    const { container } = render(<SessionPanel activeWorkspaceId="terminal3" />)

    fireEvent.click(screen.getByRole('button', { name: 'Session creation options' }))
    expect(document.querySelectorAll('.floating-panel-dismiss-layer')).toHaveLength(1)
    fireEvent.click(screen.getByRole('button', { name: /New named session/i }))
    const popup = screen.getByRole('dialog', { name: /Create named tmux session/i })
    expect(popup).toHaveClass('session-named-popup')
    const layers = document.querySelectorAll('.floating-panel-dismiss-layer')
    expect(layers).toHaveLength(1)
    expect(document.querySelectorAll('.session-context-menu')).toHaveLength(1)
    expect(Number(popup.style.zIndex)).toBeGreaterThan(Number((layers[0] as HTMLElement).style.zIndex))
    expect(container.querySelector('.session-panel-content .session-named-create')).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('New session name'), { target: { value: 'research-agent' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create named session' }))

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({ workspaceId: 'terminal3', unixUser: 'bob', name: 'research-agent' })
    expect(screen.queryByText('Nuke All')).not.toBeInTheDocument()
  })

  it('keeps bulk destruction out of the primary Session panel', () => {
    mockState.sessions.push({ name: 'shell', windows: 1, attached: false, group: 'shell' })
    render(<SessionPanel activeWorkspaceId="terminal1" />)
    expect(screen.queryByText('Nuke All')).not.toBeInTheDocument()
  })
})
