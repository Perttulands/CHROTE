import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SendToSessionRequest, TerminalWorkspace, TmuxSession, UserSettings, WorkspaceId } from '../types'
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
  request: null as SendToSessionRequest | null,
  requestId: 0,
  sessions: [] as TmuxSession[],
  focusedWindowKey: 'terminal1-terminal1-window-0' as string | null,
  focusedSession: 'alice:alice-shell' as string | null,
  closeSendToSession: vi.fn(),
  listSessionPanes: vi.fn(),
  sendToSession: vi.fn(),
  createSession: vi.fn(),
  scrollToBottom: vi.fn(),
  settings: null as UserSettings | null,
  updateSettings: vi.fn(),
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
    settings: mockState.settings ?? DEFAULT_SETTINGS,
    updateSettings: mockState.updateSettings,
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

vi.mock('../context/StatusContext', () => ({ useStatus: () => ({ announce: vi.fn() }) }))

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
    mockState.settings = null
    mockState.request = null
    mockState.requestId = 0
    mockState.sessions = [...ALL_SESSIONS]
    mockState.focusedWindowKey = 'terminal1-terminal1-window-0'
    mockState.focusedSession = 'alice:alice-shell'
    mockState.listSessionPanes.mockResolvedValue([RESOLVED_PANE])
    mockState.sendToSession.mockResolvedValue({ outcome: 'sent', message: "Pasted to 'alice-shell' (%1)" })
    mockState.createSession.mockResolvedValue('shell-home')
  })

  it('creates a scheduled prompt directly for the menu session', async () => {
    const fetcher = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ data: { task: { id: 'tsk_1', nextRun: '2026-10-12T09:00:00Z' } } })))
    open({ targetSessionKey: 'build:forge', scheduledTask: true })
    render(<SendDrawer />)
    fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: 'Review build failures' } })
    fireEvent.click(screen.getByRole('button', { name: 'Every' }))
    fireEvent.change(screen.getByLabelText('Interval'), { target: { value: '45' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create task' }))
    await waitFor(() => expect(mockState.closeSendToSession).toHaveBeenCalled())
    expect(fetcher).toHaveBeenCalledWith('/api/scheduled-tasks', expect.objectContaining({
      method: 'POST', headers: expect.objectContaining({ 'X-Chrote-Intent': 'scheduled-task' }),
    }))
    const body = JSON.parse(fetcher.mock.calls[0][1]!.body as string)
    expect(body).toMatchObject({ prompt: 'Review build failures', targets: [{ sessionName: 'forge', unixUser: 'build' }], schedule: { type: 'interval', everyMinutes: 45 }, enabled: true, paused: false })
    fetcher.mockRestore()
  })

  it('sends the schedule and setup instructions through the exact pane for refinement', async () => {
    open({ targetSessionKey: 'alice:codex-agent', scheduledTask: true })
    render(<SendDrawer />)
    fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: 'Review blockers' } })
    fireEvent.click(screen.getByRole('checkbox', { name: 'Have agent refine' }))
    const send = screen.getByRole('button', { name: 'Send to agent' })
    await waitFor(() => expect(send).toBeEnabled())
    fireEvent.click(send)
    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalledWith('codex-agent', expect.objectContaining({
      text: expect.stringContaining('X-Chrote-Intent: scheduled-task'), submit: true,
      pane: '%1', sessionId: '$1', panePid: '101', serverPid: '9001',
    }), 'alice'))
    const text = mockState.sendToSession.mock.calls[0][1].text
    expect(text).toContain('Review blockers')
    expect(text).toContain('"sessionName": "codex-agent"')
    expect(text).toContain('"expression": "0 9 * * *"')
    expect(text).toContain('/api/scheduled-tasks')
  })

  it('keeps a failed scheduled task draft and confirms before discarding it', async () => {
    const fetcher = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { message: 'Session disappeared' } }), { status: 404 }))
    open({ targetSessionKey: 'build:forge', scheduledTask: true })
    render(<SendDrawer />)
    fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: 'Keep this draft' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create task' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Session disappeared')
    expect(screen.getByLabelText('Prompt')).toHaveValue('Keep this draft')
    fireEvent.click(screen.getByRole('button', { name: 'Close scheduled task' }))
    expect(mockState.closeSendToSession).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Discard scheduled task' }))
    expect(mockState.closeSendToSession).toHaveBeenCalled()
    fetcher.mockRestore()
  })

  // The drawer opens at whatever width this device last gave it, and the
  // handle's arrow step hands the new width back to be remembered.
  it('opens at the width it remembers and commits the width the handle sets', async () => {
    mockState.settings = { ...DEFAULT_SETTINGS, sendDrawerWidth: 420 }
    await renderOpen({ targetSessionKey: 'alice:alice-shell' })

    expect(screen.getByRole('dialog', { name: 'Send to session' })).toHaveStyle({ width: '420px' })

    fireEvent.keyDown(screen.getByRole('separator', { name: 'Resize Send drawer' }), { key: 'ArrowLeft' })
    expect(mockState.updateSettings).toHaveBeenCalledWith({ sendDrawerWidth: 436 })
  })

  it('falls back to its default width when nothing usable is remembered', async () => {
    mockState.settings = { ...DEFAULT_SETTINGS, sendDrawerWidth: Number.NaN }
    await renderOpen({ targetSessionKey: 'alice:alice-shell' })

    expect(screen.getByRole('dialog', { name: 'Send to session' })).toHaveStyle({ width: '380px' })
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

  it('pastes without submitting on Ctrl+Enter, and on the Paste action', async () => {
    await renderOpen({ targetSessionKey: 'alice:alice-shell' })

    fireEvent.change(note(), { target: { value: 'stand by' } })
    fireEvent.keyDown(note(), { key: 'Enter', ctrlKey: true })
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

  // The drawer lies over whatever opened it, and closing it hands the focus
  // back there: the card's Send button, a tile, a row.
  it('closes on Escape and returns the focus to the control that opened it', async () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()

    const { rerender } = await renderOpen({ targetSessionKey: 'alice:alice-shell' })
    expect(note()).toHaveFocus()

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(mockState.closeSendToSession).toHaveBeenCalledTimes(1)

    mockState.request = null
    rerender(<SendDrawer />)
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(opener).toHaveFocus()
    opener.remove()
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
