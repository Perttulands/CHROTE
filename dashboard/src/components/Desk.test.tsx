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
  useTerminalPool: () => ({ terminals: new Map(), connectionStates: new Map() }),
}))

vi.mock('./TerminalSurface', () => ({
  default: () => <div data-testid="terminal-surface" />,
  useTerminalSession: () => ({ session: { id: 'terminal' }, connectionState: 'open' }),
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

  it('offers the launcher on the desk folder when the session is not running', () => {
    mockState.sessions = []
    renderDesk()
    fireEvent.click(screen.getByText('Launch'))
    expect(screen.getByTestId('launcher')).toHaveTextContent('/srv/ops/tender')
  })
})
