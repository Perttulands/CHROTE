import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SessionPanel from './SessionPanel'
import { DEFAULT_SETTINGS } from '../types'
import type { TmuxSession } from '../types'

const refreshSessions = vi.fn()
const createSession = vi.fn()
const addToast = vi.fn()
const fetchMock = vi.fn()

const mockState = vi.hoisted(() => ({
  sessions: [] as Array<Record<string, unknown>>,
  groupedSessions: {} as Record<string, TmuxSession[]>,
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    groupedSessions: mockState.groupedSessions,
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
    assignedSessions: new Map(),
    handleSessionClick: vi.fn(),
    focusSessionAssignment: vi.fn(),
    deleteSession: vi.fn(),
    renameSession: vi.fn(),
    workspaces: {
      terminal1: { windows: [], windowCount: 0 },
      terminal2: { windows: [], windowCount: 0 },
      terminal3: { windows: [], windowCount: 0 },
    },
    workspaceIds: ['terminal1', 'terminal2', 'terminal3'],
    addSessionToWindow: vi.fn(),
    removeSessionFromWindow: vi.fn(),
    openFloatingModal: vi.fn(),
    openSendToSession: vi.fn(),
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
    mockState.groupedSessions = {}
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

  it('renders prioritized groups, count badges, and case-insensitive filtered values', () => {
    mockState.groupedSessions = {
      other: [{ name: 'worker-beta', windows: 1, attached: false, group: 'other' }],
      hq: [
        { name: 'MAYOR', windows: 1, attached: false, group: 'hq' },
        { name: 'deacon', windows: 1, attached: false, group: 'hq' },
      ],
    }
    const { container } = render(<SessionPanel activeWorkspaceId="terminal1" />)

    expect(Array.from(container.querySelectorAll('.group-name'), node => node.textContent)).toEqual(['HQ', 'Other'])
    expect(Array.from(container.querySelectorAll('.session-count'), node => node.textContent)).toEqual(['2', '1'])

    fireEvent.change(screen.getByPlaceholderText('Filter sessions...'), { target: { value: 'may' } })
    expect(screen.getByText('MAYOR')).toBeInTheDocument()
    expect(screen.queryByText('deacon')).not.toBeInTheDocument()
    expect(screen.queryByText('Other')).not.toBeInTheDocument()

    fireEvent.change(screen.getByPlaceholderText('Filter sessions...'), { target: { value: '' } })
    expect(screen.getByText('deacon')).toBeInTheDocument()
    expect(screen.getByText('worker-beta')).toBeInTheDocument()
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
