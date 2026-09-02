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

describe('SessionPanel session launcher', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockState.sessions.length = 0
    mockState.groupedSessions = {}
    createSession.mockResolvedValue('created')
    fetchMock.mockImplementation((input: unknown) => (
      String(input).includes('/api/launch')
        ? Promise.resolve({
          ok: true,
          json: async () => ({
            harnesses: [{ id: 'codex', label: 'Codex' }, { id: 'shell', label: 'Shell' }],
            folders: ['/srv/chrote', '~'],
          }),
        })
        : Promise.resolve({ ok: true, json: async () => ({ success: true, removed: true }) })
    ))
    vi.stubGlobal('fetch', fetchMock)
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('opens the launcher from the plus and launches into the visible terminal workspace', async () => {
    render(<SessionPanel activeWorkspaceId="terminal3" />)

    fireEvent.click(screen.getByTitle('New tmux session'))

    const popup = screen.getByRole('dialog', { name: /Launch a tmux session/i })
    expect(popup).toHaveClass('session-launcher-popup')
    expect(document.querySelectorAll('.floating-panel-dismiss-layer')).toHaveLength(1)
    const launchButton = await screen.findByRole('button', { name: 'Launch codex in chrote' })

    fireEvent.click(launchButton)

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    // No attachTo: the panel starts a session, it does not claim a tile for it.
    expect(createSession).toHaveBeenCalledWith({
      name: 'codex-chrote',
      unixUser: 'bob',
      cwd: '/srv/chrote',
      harness: 'codex',
      workspaceId: 'terminal3',
    })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: /Launch a tmux session/i })).not.toBeInTheDocument())
  })

  it('renders generic named groups before ungrouped sessions with counts and filtering', () => {
    mockState.groupedSessions = {
      other: [{ name: 'worker-beta', windows: 1, attached: false, group: 'other' }],
      project: [
        { name: 'PROJECT-API', windows: 1, attached: false, group: 'project' },
        { name: 'project-worker', windows: 1, attached: false, group: 'project' },
      ],
    }
    const { container } = render(<SessionPanel activeWorkspaceId="terminal1" />)

    expect(Array.from(container.querySelectorAll('.group-name'), node => node.textContent)).toEqual(['project', 'Other'])
    expect(Array.from(container.querySelectorAll('.session-count'), node => node.textContent)).toEqual(['2', '1'])

    fireEvent.change(screen.getByPlaceholderText('Filter sessions...'), { target: { value: 'api' } })
    // A row's name is drawn as head and tail spans, so it is found by title.
    expect(screen.getByTitle('PROJECT-API')).toBeInTheDocument()
    expect(screen.queryByTitle('project-worker')).not.toBeInTheDocument()
    expect(screen.queryByText('Other')).not.toBeInTheDocument()

    fireEvent.change(screen.getByPlaceholderText('Filter sessions...'), { target: { value: '' } })
    expect(screen.getByTitle('project-worker')).toBeInTheDocument()
    expect(screen.getByTitle('worker-beta')).toBeInTheDocument()
  })

  it('launches as the Unix user the operator picks in the launcher', async () => {
    render(<SessionPanel activeWorkspaceId="terminal2" />)

    fireEvent.click(screen.getByTitle('New tmux session'))
    fireEvent.click(await screen.findByRole('button', { name: 'bob' }))
    fireEvent.click(screen.getByRole('button', { name: 'Launch codex in chrote' }))

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({
      name: 'codex-chrote',
      unixUser: 'bob',
      cwd: '/srv/chrote',
      harness: 'codex',
      workspaceId: 'terminal2',
    })
  })

  it('launches the exact name the operator types over the derived one', async () => {
    render(<SessionPanel activeWorkspaceId="terminal3" />)

    fireEvent.click(screen.getByTitle('New tmux session'))
    const nameField = await screen.findByLabelText('Session name')
    expect(nameField).toHaveValue('codex-chrote')
    fireEvent.change(nameField, { target: { value: 'research-agent' } })
    fireEvent.click(screen.getByRole('button', { name: 'Launch codex in chrote' }))

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({
      name: 'research-agent',
      unixUser: 'bob',
      cwd: '/srv/chrote',
      harness: 'codex',
      workspaceId: 'terminal3',
    })
    expect(screen.queryByText('Nuke All')).not.toBeInTheDocument()
  })

  it('keeps bulk destruction out of the primary Session panel', () => {
    mockState.sessions.push({ name: 'shell', windows: 1, attached: false, group: 'shell' })
    render(<SessionPanel activeWorkspaceId="terminal1" />)
    expect(screen.queryByText('Nuke All')).not.toBeInTheDocument()
  })
})
