import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SessionPanel from './SessionPanel'
import { DEFAULT_SETTINGS } from '../types'

const refreshSessions = vi.fn()
const addToast = vi.fn()

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    groupedSessions: {},
    loading: false,
    error: null,
    sidebarCollapsed: false,
    toggleSidebar: vi.fn(),
    refreshSessions,
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
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({ ok: true })) as any)
  })

  it('creates a new session as the selected configured Unix user from the New Session context menu', async () => {
    render(<SessionPanel />)

    fireEvent.contextMenu(screen.getByTitle('New tmux session'))
    fireEvent.click(screen.getByRole('button', { name: /New as B bob/i }))

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled())
    const [, init] = (globalThis.fetch as any).mock.calls[0]
    expect(JSON.parse(init.body)).toMatchObject({ unixUser: 'bob' })
    expect(JSON.parse(init.body).name).toMatch(/^bob\d+$/)
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

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled())
    const [, init] = (globalThis.fetch as any).mock.calls[0]
    expect(JSON.parse(init.body)).toMatchObject({ name: 'research-agent', unixUser: 'alice' })
  })
})
