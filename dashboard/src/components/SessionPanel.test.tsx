import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SessionPanel from './SessionPanel'
import { DEFAULT_SETTINGS } from '../types'

const refreshSessions = vi.fn()
const createSession = vi.fn()
const addToast = vi.fn()
const fetchMock = vi.fn()

const mockState = vi.hoisted(() => ({
  sessionBank: [] as Array<Record<string, unknown>>,
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
    sessions: [],
    sessionBank: mockState.sessionBank,
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

function addAgentBankedSession(overrides: Record<string, unknown> = {}) {
  mockState.sessionBank.push({
    id: '$7',
    name: 'codex-alpha',
    unixUser: 'alice',
    windows: 1,
    attached: false,
    group: 'codex',
    live: false,
    firstSeen: '2026-07-07T20:00:00Z',
    lastSeen: '2026-07-07T21:00:00Z',
    recoveryKind: 'agent',
    agentKind: 'codex',
    agentSessionId: '019f45ec-f88b-7f70-88dc-b5b99a9e94c6',
    resumeCommand: 'codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6',
    cwd: '/srv/chrote',
    ...overrides,
  })
}

function addShellBankedSession(overrides: Record<string, unknown> = {}) {
  mockState.sessionBank.push({
    id: '$8',
    name: 'shell-archive',
    unixUser: 'alice',
    windows: 1,
    attached: false,
    group: 'shell',
    live: false,
    firstSeen: '2026-07-07T20:00:00Z',
    lastSeen: '2026-07-07T21:00:00Z',
    recoveryKind: 'shell',
    ...overrides,
  })
}

describe('SessionPanel new-session context menu', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockState.sessionBank.length = 0
    createSession.mockResolvedValue('created')
    fetchMock.mockResolvedValue({ ok: true, json: async () => ({ success: true, removed: true }) })
    vi.stubGlobal('fetch', fetchMock)
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('creates a default side-panel session through the shared creation action', async () => {
    render(<SessionPanel />)

    fireEvent.click(screen.getByTitle('New tmux session'))

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({ workspaceId: 'terminal1' })
  })

  it('creates a new session as the selected configured Unix user from the New Session context menu', async () => {
    render(<SessionPanel />)

    fireEvent.contextMenu(screen.getByTitle('New tmux session'))
    fireEvent.click(screen.getByRole('button', { name: /New as B bob/i }))

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({ workspaceId: 'terminal1', unixUser: 'bob' })
  })

  it('opens a named session field and creates the exact typed session name', async () => {
    const { container } = render(<SessionPanel />)

    fireEvent.contextMenu(screen.getByTitle('New tmux session'))
    fireEvent.click(screen.getByRole('button', { name: /New named session/i }))
    const popup = screen.getByRole('dialog', { name: /Create named tmux session/i })
    expect(popup).toHaveClass('session-named-popup')
    expect(container.querySelector('.session-panel-content .session-named-create')).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('New session name'), { target: { value: 'research-agent' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create named session' }))

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({ workspaceId: 'terminal1', unixUser: 'alice', name: 'research-agent' })
  })

  it('shows recoverable agent bank entries with resume, copy, recreate, and remove actions', async () => {
    addAgentBankedSession()

    render(<SessionPanel />)

    expect(screen.getByRole('heading', { name: 'Session bank' })).toBeInTheDocument()
    expect(screen.getByText('codex-alpha')).toBeInTheDocument()
    expect(screen.getByText('Recoverable agent')).toBeInTheDocument()
    expect(screen.getByText(/codex · 019f45ec-f88b-7f70-88dc-b5b99a9e94c6/)).toBeInTheDocument()
    expect(screen.getByText('codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Copy resume command for codex-alpha' }))
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith('codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6'))
    expect(addToast).toHaveBeenCalledWith('Resume command copied', 'success')

    fireEvent.click(screen.getByRole('button', { name: 'Recreate shell for codex-alpha' }))
    await waitFor(() => expect(createSession).toHaveBeenCalledWith({ workspaceId: 'terminal1', unixUser: 'alice', name: 'codex-alpha' }))
  })

  it('resumes a recoverable agent through the session-bank recovery API', async () => {
    addAgentBankedSession()

    render(<SessionPanel />)

    fireEvent.click(screen.getByRole('button', { name: 'Resume agent for codex-alpha' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const [url, options] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/tmux/session-bank/codex-alpha/recover?unixUser=alice')
    expect(options).toMatchObject({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mouseScroll: DEFAULT_SETTINGS.mouseScroll }),
    })
    expect(addToast).toHaveBeenCalledWith('Resumed agent codex-alpha', 'success')
    expect(refreshSessions).toHaveBeenCalled()
  })

  it('shows shell-only bank entries without agent-only actions', () => {
    addShellBankedSession()

    render(<SessionPanel />)

    expect(screen.getByText('shell-archive')).toBeInTheDocument()
    expect(screen.getByText('Shell only')).toBeInTheDocument()
    expect(screen.getByText('No agent resume metadata saved.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Resume agent for shell-archive' })).not.toBeInTheDocument()
    expect(screen.getByText('/resume shell-archive')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy resume command for shell-archive' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Recreate shell for shell-archive' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Remove shell-archive from session bank' })).toBeInTheDocument()
  })

  it('collapses and expands the offline session bank list', () => {
    addAgentBankedSession()

    render(<SessionPanel />)

    expect(screen.getByText('codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Collapse session bank' }))

    expect(screen.queryByText('codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Expand session bank' })).toHaveAttribute('aria-expanded', 'false')

    fireEvent.click(screen.getByRole('button', { name: 'Expand session bank' }))
    expect(screen.getByText('codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6')).toBeInTheDocument()
  })

  it('removes an offline banked session from the durable bank list', async () => {
    addAgentBankedSession()

    render(<SessionPanel />)

    fireEvent.click(screen.getByRole('button', { name: 'Remove codex-alpha from session bank' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const [url, options] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/tmux/session-bank/codex-alpha?unixUser=alice')
    expect(options).toMatchObject({ method: 'DELETE' })
    expect(addToast).toHaveBeenCalledWith('Removed codex-alpha from session bank', 'success')
    expect(refreshSessions).toHaveBeenCalled()
  })
})
