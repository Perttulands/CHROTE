import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Desk from './Desk'
import { DEFAULT_SETTINGS, type TmuxSession } from '../types'

const mockState = vi.hoisted(() => ({
  announce: vi.fn(),
  sendToSession: vi.fn(),
  openSendToSession: vi.fn(),
  refreshSessions: vi.fn(),
  sessions: [] as TmuxSession[],
  pooled: new Map<string, unknown>(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sessions: mockState.sessions,
    settings: DEFAULT_SETTINGS,
    openSendToSession: mockState.openSendToSession,
    sendToSession: mockState.sendToSession,
    refreshSessions: mockState.refreshSessions,
  }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('./TerminalPool', () => ({
  useTerminalPool: () => ({ terminals: mockState.pooled, connectionStates: new Map() }),
}))

vi.mock('./TerminalSurface', () => ({
  default: ({ session }: { session: { url?: string } | null }) => (
    <div data-testid="terminal-surface" data-url={session?.url ?? ''} />
  ),
  useTerminalSession: (url: string | null) => ({
    session: url ? { url } : null,
    connectionState: url ? 'open' : 'idle',
  }),
}))

vi.mock('./Launcher', () => ({
  default: ({ initialFolder }: { initialFolder?: string }) => <div data-testid="launcher">{initialFolder}</div>,
}))

function session(overrides: Partial<TmuxSession> = {}): TmuxSession {
  return {
    name: 'tender',
    windows: 1,
    attached: false,
    group: 'default',
    unixUser: 'operator',
    currentCommand: 'claude',
    ...overrides,
  }
}

function renderDesk(props: Partial<React.ComponentProps<typeof Desk>> = {}) {
  return render(
    <Desk
      label="Tender"
      sessionName="tender"
      reference="agents /srv/chrote claude-code"
      placeholder="Ask the tender…"
      launchFolder="/srv/ops/tender"
      {...props}
    />,
  )
}

describe('Desk', () => {
  beforeEach(() => {
    mockState.announce.mockReset()
    mockState.openSendToSession.mockReset()
    mockState.refreshSessions.mockReset()
    mockState.sendToSession.mockReset()
    mockState.sendToSession.mockResolvedValue({ outcome: 'sent', message: 'sent' })
    mockState.sessions = [session()]
    mockState.pooled = new Map<string, unknown>()
    window.localStorage.clear()
  })

  it.each([
    { name: 'a session with a client attached is live', sessions: [session({ attached: true })], word: 'live' },
    { name: 'a session with none is idle', sessions: [session({ attached: false })], word: 'idle' },
    { name: 'a session tmux does not have is not running', sessions: [], word: 'not running' },
  ])('$name', ({ sessions, word }) => {
    mockState.sessions = sessions
    renderDesk()
    expect(screen.getByText(word)).toBeInTheDocument()
  })

  it('says so when the host configured no session at all', () => {
    renderDesk({ sessionName: undefined })
    expect(screen.getByText('not configured')).toBeInTheDocument()
    expect(screen.queryByLabelText('Ask tender')).not.toBeInTheDocument()
    expect(screen.queryByText('Expand')).not.toBeInTheDocument()
  })

  it('sends the reference above the question and says who was asked', async () => {
    renderDesk()
    const field = screen.getByLabelText('Ask tender')
    fireEvent.change(field, { target: { value: 'what changed in the doctrine?' } })
    fireEvent.keyDown(field, { key: 'Enter' })

    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalledWith(
      'tender',
      { text: 'agents /srv/chrote claude-code\nwhat changed in the doctrine?', files: [], submit: true },
      'operator',
    ))
    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith('Asked tender', 'success'))
    expect(field).toHaveValue('')
  })

  it('keeps a question that did not land', async () => {
    mockState.sendToSession.mockResolvedValue({ outcome: 'failed', message: 'No such pane' })
    renderDesk()
    const field = screen.getByLabelText('Ask tender')
    fireEvent.change(field, { target: { value: 'still worth asking' } })
    fireEvent.keyDown(field, { key: 'Enter' })

    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalled())
    expect(field).toHaveValue('still worth asking')
  })

  it('hands Alt+S to the drawer with the same session and reference', () => {
    renderDesk()
    const field = screen.getByLabelText('Ask tender')
    fireEvent.change(field, { target: { value: 'a longer note' } })
    fireEvent.keyDown(field, { key: 's', altKey: true })

    expect(mockState.openSendToSession).toHaveBeenCalledWith({
      targetSessionKey: 'operator:tender',
      reference: 'agents /srv/chrote claude-code',
      note: 'a longer note',
    })
    expect(mockState.sendToSession).not.toHaveBeenCalled()
  })

  it('expands into the session terminal and collapses again', () => {
    renderDesk()
    fireEvent.click(screen.getByText('Expand'))
    expect(screen.getByTestId('terminal-surface')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Collapse'))
    expect(screen.queryByTestId('terminal-surface')).not.toBeInTheDocument()
  })

  /*
   * The pool's terminal belongs to the tile that bound it, and a terminal is
   * attached in one place at a time. A desk that took the pooled one would
   * empty that tile behind the operator's back, so it dials its own.
   */
  it('dials its own terminal rather than taking the one a tile is showing', () => {
    mockState.pooled = new Map<string, unknown>([['operator:tender', { url: 'the tile own terminal' }]])
    renderDesk()

    fireEvent.click(screen.getByText('Expand'))

    expect(screen.getByTestId('terminal-surface')).not.toHaveAttribute('data-url', 'the tile own terminal')
  })

  it('remembers the expansion for that desk and session', () => {
    const first = renderDesk()
    fireEvent.click(screen.getByText('Expand'))
    first.unmount()

    renderDesk()
    expect(screen.getByTestId('terminal-surface')).toBeInTheDocument()
  })

  it('offers the launcher on the desk folder when the session is not running', () => {
    mockState.sessions = []
    renderDesk()
    fireEvent.click(screen.getByText('Launch'))
    expect(screen.getByTestId('launcher')).toHaveTextContent('/srv/ops/tender')
  })
})
