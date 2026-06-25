import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SessionPanel from './SessionPanel'
import { DEFAULT_SETTINGS } from '../types'

const refreshSessions = vi.fn()
const createSession = vi.fn()
const addToast = vi.fn()

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
    createSession.mockResolvedValue('created')
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
})
