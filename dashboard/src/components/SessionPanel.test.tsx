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

  it('keeps banked session cards out of the Terminal tab and exposes only a compact Settings affordance', () => {
    addAgentBankedSession()
    const openSessionBankSettings = vi.fn()

    render(<SessionPanel onOpenSessionBankSettings={openSessionBankSettings} />)

    expect(screen.queryByRole('heading', { name: /Session bank/i })).not.toBeInTheDocument()
    expect(screen.queryByText('codex-alpha')).not.toBeInTheDocument()
    expect(screen.queryByText('codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6')).not.toBeInTheDocument()

    const settingsLink = screen.getByRole('button', { name: 'Open Session Bank settings for 1 recoverable session' })
    expect(settingsLink).toHaveTextContent('Session Bank · 1 recoverable')
    fireEvent.click(settingsLink)
    expect(openSessionBankSettings).toHaveBeenCalled()
  })
})
