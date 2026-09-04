import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ResidentColumn from './ResidentColumn'
import { resetChordsForTest } from '../keys/chords'
import { resetResidentForTest } from '../residents/residentPresence'
import { resetResidentsForTest, type Resident } from '../residents/residentsApi'
import { resetTableForTest } from '../context/TableContext'
import { DEFAULT_SETTINGS, type TmuxSession } from '../types'

const mockState = vi.hoisted(() => ({
  announce: vi.fn(),
  sendToSession: vi.fn(),
  openSendToSession: vi.fn(),
  openFloatingModal: vi.fn(),
  deleteSession: vi.fn(),
  refreshSessions: vi.fn(),
  updateSettings: vi.fn(),
  sessions: [] as TmuxSession[],
  residents: [] as Resident[],
  focused: [] as string[],
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sessions: mockState.sessions,
    settings: DEFAULT_SETTINGS,
    updateSettings: mockState.updateSettings,
    openSendToSession: mockState.openSendToSession,
    openFloatingModal: mockState.openFloatingModal,
    sendToSession: mockState.sendToSession,
    deleteSession: mockState.deleteSession,
    refreshSessions: mockState.refreshSessions,
  }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('../residents/residentsApi', async () => {
  const actual = await vi.importActual<typeof import('../residents/residentsApi')>('../residents/residentsApi')
  return { ...actual, fetchResidents: () => Promise.resolve(mockState.residents) }
})

vi.mock('./TerminalSurface', () => ({
  default: ({ session }: { session: { url?: string } | null }) => (
    <div data-testid="terminal-surface" data-url={session?.url ?? ''} />
  ),
  useTerminalSession: (url: string | null) => ({
    session: url ? { url, focus: () => mockState.focused.push(url), scrollToBottom: () => {} } : null,
    connectionState: url ? 'open' : 'idle',
  }),
}))

vi.mock('./Launcher', () => ({
  default: ({ initialFolder, initialName }: { initialFolder?: string; initialName?: string }) => (
    <div data-testid="launcher">{`${initialName ?? ''} in ${initialFolder ?? ''}`}</div>
  ),
}))

function session(overrides: Partial<TmuxSession> = {}): TmuxSession {
  return {
    name: 'clerk',
    windows: 1,
    attached: false,
    group: 'default',
    unixUser: 'operator',
    currentCommand: 'claude',
    ...overrides,
  }
}

const CLERK: Resident = { tab: 'beads', label: 'Clerk', session: 'clerk', folder: '/work/clerk', beads: '/work' }

async function renderColumn(reference: string | null = 'bead test-1: Fix login bug') {
  const rendered = render(<ResidentColumn tab="beads" reference={reference} />)
  await waitFor(() => expect(screen.getByText(/live|idle|not running|not configured/)).toBeInTheDocument())
  return rendered
}

beforeEach(() => {
  mockState.announce.mockReset()
  mockState.sendToSession.mockReset()
  mockState.openSendToSession.mockReset()
  mockState.openFloatingModal.mockReset()
  mockState.deleteSession.mockReset()
  mockState.refreshSessions.mockReset()
  mockState.updateSettings.mockReset()
  mockState.sendToSession.mockResolvedValue({ outcome: 'sent', message: 'sent' })
  mockState.sessions = [session()]
  mockState.residents = [CLERK]
  mockState.focused = []
  window.localStorage.clear()
})

afterEach(() => {
  resetChordsForTest()
  resetResidentForTest()
  resetResidentsForTest()
  resetTableForTest()
})

describe('the resident column', () => {
  it.each([
    { name: 'a session with a client attached is live', sessions: [session({ attached: true })], word: 'live' },
    { name: 'a session with none is idle', sessions: [session({ attached: false })], word: 'idle' },
    { name: 'a session tmux does not have is not running', sessions: [], word: 'not running' },
  ])('$name', async ({ sessions, word }) => {
    mockState.sessions = sessions
    await renderColumn()
    expect(screen.getByText(word)).toBeInTheDocument()
  })

  it('says so when the host configured no session, and shows nothing else', async () => {
    mockState.residents = [{ ...CLERK, session: '' }]
    await renderColumn()
    expect(screen.getByText('not configured')).toBeInTheDocument()
    expect(screen.queryByTestId('terminal-surface')).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('shows the session live beneath its header', async () => {
    await renderColumn()
    expect(screen.getByRole('complementary', { name: 'The Clerk' })).toHaveTextContent('clerk')
    expect(screen.getByTestId('terminal-surface')).toBeInTheDocument()
  })

  // The paste text is the whole of what the agent is handed: the reference on
  // a line of its own, nothing submitted, and the keyboard left in the prompt.
  it('pastes the reference into the prompt without submitting, then focuses the terminal', async () => {
    await renderColumn()
    fireEvent.click(screen.getByRole('button', { name: /^Send/ }))

    await waitFor(() => expect(mockState.sendToSession).toHaveBeenCalledWith(
      'clerk',
      { text: 'bead test-1: Fix login bug\n', files: [], submit: false },
      'operator',
    ))
    await waitFor(() => expect(mockState.focused).toHaveLength(1))
    expect(mockState.openSendToSession).not.toHaveBeenCalled()
  })

  it('hands Alt+S to the drawer with the reference when the session is not running', async () => {
    mockState.sessions = []
    await renderColumn()
    fireEvent.keyDown(document, { key: 's', altKey: true })

    expect(mockState.openSendToSession).toHaveBeenCalledWith({ reference: 'bead test-1: Fix login bug' })
    expect(mockState.sendToSession).not.toHaveBeenCalled()
  })

  it('offers Launch with the resident own folder and name when the session is absent', async () => {
    mockState.sessions = []
    await renderColumn()
    fireEvent.click(screen.getByRole('button', { name: 'Launch' }))
    expect(screen.getByTestId('launcher')).toHaveTextContent('clerk in /work/clerk')
  })

  it('collapses to its header and remembers it for the tab', async () => {
    const first = await renderColumn()
    fireEvent.click(screen.getByRole('button', { name: 'Collapse' }))
    expect(screen.queryByTestId('terminal-surface')).toBeNull()
    first.unmount()

    await renderColumn()
    expect(screen.queryByTestId('terminal-surface')).toBeNull()
    expect(screen.getByRole('button', { name: 'Expand' })).toBeInTheDocument()
  })
})
