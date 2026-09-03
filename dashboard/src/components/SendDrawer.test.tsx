import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SendToSessionRequest, TerminalWorkspace, TmuxSession, WorkspaceId } from '../types'
import { DEFAULT_SETTINGS } from '../types'
import { sessionEvidenceFrom } from '../terminal/tileState'
import SendDrawer, { composeMessage } from './SendDrawer'

const RESOLVED_PANE = {
  sessionId: '$1',
  pane: '%1',
  panePid: '101',
  serverPid: '9001',
  windowName: 'main',
  currentCommand: 'bash',
  currentPath: '/home/alice',
  active: true,
}

const ALL_SESSIONS: TmuxSession[] = [
  { name: 'alice-shell', windows: 1, attached: true, group: 'main', unixUser: 'alice' },
  { name: 'codex-agent', windows: 1, attached: true, group: 'codex', unixUser: 'alice' },
  { name: 'forge', windows: 1, attached: true, group: 'build', unixUser: 'build' },
]

function workspace(activeSession: string | null): TerminalWorkspace {
  return {
    windowCount: 1,
    windows: [{ id: 'terminal1-window-0', boundSessions: activeSession ? [activeSession] : [], activeSession, colorIndex: 0 }],
  }
}

const mockState = vi.hoisted(() => ({
  request: null as { targetSessionKey?: string; reference?: string; note?: string } | null,
  requestId: 0,
  sessions: [] as TmuxSession[],
  focusedWindowKey: 'terminal1-terminal1-window-0' as string | null,
  focusedSession: 'alice:alice-shell' as string | null,
  closeSendToSession: vi.fn(),
  listSessionPanes: vi.fn(),
  sendToSession: vi.fn(),
  createSession: vi.fn(),
  scrollToBottom: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sendToSessionRequest: mockState.request,
    sendToSessionRequestId: mockState.requestId,
    sessions: mockState.sessions,
    workspaces: { terminal1: workspace(mockState.focusedSession) } as Record<WorkspaceId, TerminalWorkspace>,
    workspaceIds: ['terminal1'] as WorkspaceId[],
    focusedWindowKey: mockState.focusedWindowKey,
    terminalUsers: ['alice', 'build'],
    settings: DEFAULT_SETTINGS,
    createSession: mockState.createSession,
    sessionEvidence: sessionEvidenceFrom({
      sessions: mockState.sessions,
      loading: false,
      error: null,
      partialAnsweringUsers: null,
    }),
    closeSendToSession: mockState.closeSendToSession,
    listSessionPanes: mockState.listSessionPanes,
    sendToSession: mockState.sendToSession,
  }),
}))

vi.mock('./TerminalPool', () => ({
  useTerminalPool: () => ({
    terminals: new Map([['alice:alice-shell', { scrollToBottom: mockState.scrollToBottom }]]),
    connectionStates: new Map(),
  }),
}))

function open(request: SendToSessionRequest) {
  mockState.request = request
  mockState.requestId += 1
}

function note(): HTMLTextAreaElement {
  return screen.getByLabelText('Message to send') as HTMLTextAreaElement
}

async function renderOpen(request: SendToSessionRequest) {
  open(request)
  const view = render(<SendDrawer />)
  await waitFor(() => expect(mockState.listSessionPanes).toHaveBeenCalled())
  return view
}

describe('SendDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockState.request = null
    mockState.requestId = 0
    mockState.sessions = [...ALL_SESSIONS]
    mockState.focusedWindowKey = 'terminal1-terminal1-window-0'
    mockState.focusedSession = 'alice:alice-shell'
    mockState.listSessionPanes.mockResolvedValue([RESOLVED_PANE])
    mockState.sendToSession.mockResolvedValue({ outcome: 'sent', message: "Pasted to 'alice-shell' (%1)" })
    mockState.createSession.mockResolvedValue('shell-home')
  })

  it('draws nothing until a surface opens it', () => {
    const { container } = render(<SendDrawer />)
    expect(container).toBeEmptyDOMElement()
  })

  // Each entry point names what the operator was looking at, and the drawer
  // shows that line above the note rather than folding it into the draft.
  it.each([
    { from: 'the file viewer', reference: 'path /srv/chrote/README.md' },
    { from: 'a Bead card', reference: 'bead chrote-5grx.13: The Send to Session drawer' },
    { from: 'a Library page', reference: 'library preferences/tools.md' },
  ])('shows the reference $from passed, read-only, above the note', async ({ reference }) => {
    await renderOpen({ targetSessionKey: 'alice:alice-shell', reference })

    expect(screen.getByText(reference)).toBeInTheDocument()
    expect(note()).toHaveValue('')
    expect(note()).toHaveFocus()
  })

  it('takes the entry point\'s target, and the focused tile\'s session when it names none', async () => {
    const { unmount } = await renderOpen({ targetSessionKey: 'build:forge' })
    expect(screen.getByRole('option', { name: /forge/ })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('option', { name: /alice-shell/ })).toHaveAttribute('aria-selected', 'false')
    unmount()

    await renderOpen({})
    expect(screen.getByRole('option', { name: /alice-shell/ })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByText('focused tile')).toBeInTheDocument()
  })

  it('filters the picker to the sessions the search names', async () => {
    await renderOpen({})

    fireEvent.change(screen.getByLabelText('Search sessions'), { target: { value: 'codex' } })

    expect(screen.getByRole('option', { name: /codex-agent/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /alice-shell/ })).not.toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'New agent…' })).toBeInTheDocument()
  })

  it('sends the reference and the note on Enter, and closes onto the target tile', async () => {
    await renderOpen({ targetSessionKey: 'alice:alice-shell', reference: 'path /srv/chrote/README.md' })

    fireEvent.change(note(), { target: { value: 'read this first' } })
    fireEvent.keyDown(note(), { key: 'Enter' })

    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalledTimes(1))
    expect(mockState.sendToSession).toHaveBeenCalledWith('alice-shell', expect.objectContaining({
      text: 'path /srv/chrote/README.md\n\nread this first',
      submit: true,
      pane: '%1',
      sessionId: '$1',
      panePid: '101',
      serverPid: '9001',
    }), 'alice')
    await waitFor(() => expect(mockState.closeSendToSession).toHaveBeenCalled())
    expect(mockState.scrollToBottom).toHaveBeenCalled()
  })

  it('pastes without submitting on Shift+Enter, and on the Paste action', async () => {
    await renderOpen({ targetSessionKey: 'alice:alice-shell' })

    fireEvent.change(note(), { target: { value: 'stand by' } })
    fireEvent.keyDown(note(), { key: 'Enter', shiftKey: true })
    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalledTimes(1))
    expect(mockState.sendToSession.mock.calls[0][1]).toMatchObject({ text: 'stand by', submit: false })

    fireEvent.click(screen.getByRole('button', { name: 'Paste' }))
    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalledTimes(2))
    expect(mockState.sendToSession.mock.calls[1][1]).toMatchObject({ submit: false })
  })

  // A failure is the operator's to act on, and the note he wrote is still
  // worth something, so neither the drawer nor the draft goes anywhere.
  it('keeps a failed send in the drawer with the server\'s own words', async () => {
    mockState.sendToSession.mockResolvedValue({
      outcome: 'failed',
      message: 'pane %1 is no longer in session alice-shell',
    })
    await renderOpen({ targetSessionKey: 'alice:alice-shell' })

    fireEvent.change(note(), { target: { value: 'try this' } })
    fireEvent.keyDown(note(), { key: 'Enter' })

    expect(await screen.findByRole('alert'))
      .toHaveTextContent('pane %1 is no longer in session alice-shell')
    expect(mockState.closeSendToSession).not.toHaveBeenCalled()
    expect(note()).toHaveValue('try this')
  })

  it('refuses an ended target, and does not ask the host for its panes', async () => {
    mockState.sessions = ALL_SESSIONS.filter(session => session.name !== 'alice-shell')
    mockState.focusedSession = null
    open({ targetSessionKey: 'alice:alice-shell' })
    render(<SendDrawer />)

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'alice-shell ended. There is no session left to send to; restart it from its tile first.',
    )
    expect(mockState.listSessionPanes).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
  })

  // A session that did not exist a moment ago may still be starting, so the
  // message is left on its line for the operator to submit himself.
  it('launches a new agent and pastes the message into it without submitting', async () => {
    const fetchMock = vi.fn(() => Promise.resolve({ ok: false, json: async () => ({}) }))
    vi.stubGlobal('fetch', fetchMock)
    await renderOpen({ targetSessionKey: 'alice:alice-shell', reference: 'bead chrote-5grx.13: the drawer' })

    fireEvent.change(note(), { target: { value: 'take this one next' } })
    fireEvent.click(screen.getByRole('option', { name: 'New agent…' }))

    const launch = await screen.findByRole('button', { name: /^Open shell in/ })
    fireEvent.click(launch)

    await waitFor(() => expect(mockState.createSession).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalledTimes(1))
    const [name, payload] = mockState.sendToSession.mock.calls[0]
    expect(name).toBe('shell-home')
    expect(payload).toMatchObject({
      text: 'bead chrote-5grx.13: the drawer\n\ntake this one next',
      submit: false,
    })
    vi.unstubAllGlobals()
  })

  // Wide enough, and the drawer takes its column out of the grid rather than
  // covering the tile the message is addressed to.
  it('takes a column from the grid when docked, and overlays when there is no room', async () => {
    const { unmount } = await renderOpen({ targetSessionKey: 'alice:alice-shell' })
    expect(screen.getByRole('dialog', { name: 'Send to session' })).toHaveStyle({ width: '380px' })
    unmount()

    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 720 })
    await renderOpen({ targetSessionKey: 'alice:alice-shell' })
    expect(screen.getByRole('dialog', { name: 'Send to session' })).toHaveStyle({ width: '50%' })
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  })
})

describe('composeMessage', () => {
  it('puts the reference first and the note under it, and drops what is absent', () => {
    expect(composeMessage('path /srv/x.ts', 'have a look')).toBe('path /srv/x.ts\n\nhave a look')
    expect(composeMessage('path /srv/x.ts', '')).toBe('path /srv/x.ts')
    expect(composeMessage(undefined, 'just this')).toBe('just this')
    expect(composeMessage('   ', 'just this')).toBe('just this')
  })
})
