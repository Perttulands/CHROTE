import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Desk, { resetDesksForTest } from './Desk'
import { DEFAULT_SETTINGS } from '../types'
import type { TmuxSession } from '../types'

const mockState = vi.hoisted(() => ({
  sessions: [] as TmuxSession[],
  openSendToSession: vi.fn(),
  createSession: vi.fn(),
  sendToSession: vi.fn(),
  announce: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sessions: mockState.sessions,
    settings: DEFAULT_SETTINGS,
    openSendToSession: mockState.openSendToSession,
    createSession: mockState.createSession,
    sendToSession: mockState.sendToSession,
  }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('./TerminalSurface', () => ({
  default: () => <div data-testid="desk-terminal" />,
  useTerminalSession: (url: string | null) => ({ session: url ? { url } : null, connectionState: 'idle' }),
}))

function session(overrides: Partial<TmuxSession> & { name: string }): TmuxSession {
  return { windows: 1, attached: false, group: 'other', ...overrides }
}

beforeEach(() => {
  resetDesksForTest()
  mockState.sessions = []
  mockState.openSendToSession.mockReset()
  mockState.createSession.mockReset()
  mockState.sendToSession.mockReset()
  mockState.sendToSession.mockResolvedValue({ outcome: 'sent', message: 'Pasted' })
  mockState.announce.mockReset()
})

function renderDesk(props: Partial<React.ComponentProps<typeof Desk>> = {}) {
  return render(
    <Desk
      label="Front desk"
      sessionName="librarian"
      reference="library preferences/workflow.md"
      placeholder="Ask the Librarian…"
      launchFolder="/corpus"
      {...props}
    />,
  )
}

describe('Desk', () => {
  const states: { name: string; sessions: TmuxSession[]; props: Partial<React.ComponentProps<typeof Desk>>; want: string }[] = [
    { name: 'a session with a client attached is live', sessions: [session({ name: 'librarian', attached: true })], props: {}, want: 'live' },
    { name: 'a session nobody is watching is idle', sessions: [session({ name: 'librarian' })], props: {}, want: 'idle' },
    { name: 'a configured session that is gone is not running', sessions: [], props: {}, want: 'not running' },
    { name: 'no configured session at all', sessions: [], props: { sessionName: undefined }, want: 'not configured' },
  ]

  states.forEach(({ name, sessions, props, want }) => {
    it(name, () => {
      mockState.sessions = sessions
      renderDesk(props)

      expect(screen.getByText('Front desk')).toBeInTheDocument()
      expect(screen.getByText(want)).toBeInTheDocument()
    })
  })

  it('offers no Ask field when nobody is on duty', () => {
    renderDesk({ sessionName: undefined })

    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Expand' })).not.toBeInTheDocument()
  })

  it('offers the launcher when the configured session is not running', () => {
    renderDesk()

    fireEvent.click(screen.getByRole('button', { name: 'Launch' }))

    expect(mockState.createSession).toHaveBeenCalledWith({ name: 'librarian', cwd: '/corpus' })
  })

  it('sends the reference and the question, then says so and clears the field', async () => {
    mockState.sessions = [session({ name: 'librarian', attached: true, unixUser: 'agent' })]
    renderDesk()

    const ask = screen.getByRole('textbox')
    fireEvent.change(ask, { target: { value: 'What do we know about testing?' } })
    fireEvent.keyDown(ask, { key: 'Enter' })

    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalledWith(
      'librarian',
      {
        text: 'library preferences/workflow.md\nWhat do we know about testing?',
        submit: true,
        files: [],
      },
      'agent',
    ))
    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith('Asked librarian', 'info'))
    expect(ask).toHaveValue('')
  })

  it('keeps the question when the send did not land', async () => {
    mockState.sessions = [session({ name: 'librarian', attached: true })]
    mockState.sendToSession.mockResolvedValue({ outcome: 'failed', message: 'No such pane' })
    renderDesk()

    const ask = screen.getByRole('textbox')
    fireEvent.change(ask, { target: { value: 'Still worth asking' } })
    fireEvent.keyDown(ask, { key: 'Enter' })

    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalled())
    expect(ask).toHaveValue('Still worth asking')
  })

  it('opens the drawer on this session from Alt+S in the Ask field', () => {
    mockState.sessions = [session({ name: 'librarian', attached: true, unixUser: 'agent' })]
    renderDesk()

    const ask = screen.getByRole('textbox')
    fireEvent.change(ask, { target: { value: 'a longer question' } })
    fireEvent.keyDown(ask, { key: 's', altKey: true })

    expect(mockState.openSendToSession).toHaveBeenCalledWith({
      targetSessionKey: 'agent:librarian',
      reference: 'library preferences/workflow.md',
      note: 'a longer question',
    })
    expect(mockState.sendToSession).not.toHaveBeenCalled()
  })

  it('expands into the session terminal and folds back', () => {
    mockState.sessions = [session({ name: 'librarian', attached: true })]
    renderDesk()

    fireEvent.click(screen.getByRole('button', { name: 'Expand' }))
    expect(screen.getByTestId('desk-terminal')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Collapse' }))
    expect(screen.queryByTestId('desk-terminal')).not.toBeInTheDocument()
  })

  it('remembers the expansion when the operator comes back to the tab', () => {
    mockState.sessions = [session({ name: 'librarian', attached: true })]
    const first = renderDesk()

    fireEvent.click(screen.getByRole('button', { name: 'Expand' }))
    first.unmount()

    renderDesk()
    expect(screen.getByTestId('desk-terminal')).toBeInTheDocument()
  })
})
