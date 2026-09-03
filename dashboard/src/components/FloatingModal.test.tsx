import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FloatingModal from './FloatingModal'
import { sessionEvidenceFrom } from '../terminal/tileState'
import { FakeSocket } from '../test/fakeWebSocket'

const ALICE_SHELL = { name: 'alice-shell', windows: 1, attached: true, group: 'main', unixUser: 'alice' }

const mockState = vi.hoisted(() => ({
  closeFloatingModal: vi.fn(),
  openSendToSession: vi.fn(),
  sessions: [] as { name: string; windows: number; attached: boolean; group: string; unixUser: string }[],
  loading: false,
  error: null as string | null,
  partialAnsweringUsers: null as string[] | null,
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    floatingSession: 'alice:alice-shell',
    closeFloatingModal: mockState.closeFloatingModal,
    openSendToSession: mockState.openSendToSession,
    settings: { fontSize: 14, hideScrollbar: false },
    sessions: mockState.sessions,
    loading: mockState.loading,
    error: mockState.error,
    partialAnsweringUsers: mockState.partialAnsweringUsers,
    sessionEvidence: sessionEvidenceFrom({
      sessions: mockState.sessions,
      loading: mockState.loading,
      error: mockState.error,
      partialAnsweringUsers: mockState.partialAnsweringUsers as string[] | null,
    }),
  }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ status: null, announce: () => {} }),
}))

describe('FloatingModal Send to Session action', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockState.sessions = [ALICE_SHELL]
    mockState.loading = false
    mockState.error = null
    mockState.partialAnsweringUsers = null
    FakeSocket.instances = []
    vi.stubGlobal('WebSocket', FakeSocket)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
  })

  it('offers Send from the peek sheet without closing the terminal peek', () => {
    render(<FloatingModal />)

    fireEvent.click(screen.getByRole('button', { name: 'Send' }))

    expect(mockState.openSendToSession).toHaveBeenCalledWith({ targetSessionKey: 'alice:alice-shell' })
    expect(mockState.closeFloatingModal).not.toHaveBeenCalled()
  })

  it('docks at the left edge with no backdrop, and closes on Escape but not on an outside click', () => {
    const { container } = render(<FloatingModal />)

    const sheet = container.querySelector('.sheet.sheet-left') as HTMLElement
    expect(sheet).not.toBeNull()
    expect(screen.getByText('Loading terminal…')).toBeInTheDocument()
    // No backdrop: there is nothing between the sheet and the work behind it.
    expect(document.querySelector('.floating-panel-dismiss-layer')).toBeNull()

    fireEvent.pointerDown(document.body)
    fireEvent.click(document.body)
    expect(mockState.closeFloatingModal).not.toHaveBeenCalled()

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(mockState.closeFloatingModal).toHaveBeenCalled()
  })

  it('gives a peek at an ended session the tile\'s note instead of a silent dead terminal', () => {
    mockState.sessions = []
    render(<FloatingModal />)

    // The same wording the tile uses, over the same last frame.
    expect(screen.getByText('alice-shell ended. This frame shows its last output.')).toBeInTheDocument()
    // Naming the failure is the point: no socket is opened to rediscover it.
    expect(FakeSocket.instances).toHaveLength(0)
    expect(screen.queryByText('Loading terminal…')).toBeNull()
  })

  it('says nothing about a session whose Unix user did not answer the poll', () => {
    mockState.sessions = []
    mockState.error = 'alice: tmux socket unreachable'
    mockState.partialAnsweringUsers = ['bob']
    render(<FloatingModal />)

    expect(screen.queryByText(/ended\. This frame shows its last output\./)).toBeNull()
    expect(screen.getByText('Loading terminal…')).toBeInTheDocument()
  })

  it('peeks through the same in-page terminal as a tile, on its own observer connection', () => {
    const { unmount } = render(<FloatingModal />)

    expect(document.querySelector('.terminal-surface-host .terminal-surface')).not.toBeNull()
    expect(FakeSocket.latest().url).toContain('/terminal/ws?arg=peek&arg=alice-shell&arg=alice')

    unmount()

    expect(FakeSocket.latest().readyState).toBe(FakeSocket.CLOSED)
  })
})
