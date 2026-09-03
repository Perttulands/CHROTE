import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Peek, { PEEK_FALLBACK_COLS, PEEK_HEADER_PX, peekSize } from './Peek'
import { sessionEvidenceFrom } from '../terminal/tileState'
import { FakeSocket } from '../test/fakeWebSocket'
import { resetSurfacesForTest } from '../keys/dismiss'
import { resetChordsForTest } from '../keys/chords'

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

// One pool for every render, as the real provider keeps one between reconciles.
const emptyPool = vi.hoisted(() => ({ terminals: new Map(), connectionStates: new Map() }))
vi.mock('./TerminalPool', () => ({
  useTerminalPool: () => emptyPool,
}))

describe('the size rule', () => {
  const workspace = { width: 1280, height: 800 }
  const cell = { cellWidth: 8.4, cellHeight: 17 }

  it('takes the session\'s columns and rows at the cell size, with the chrome around them', () => {
    // 80 columns at 8.4px is 672px; the terminal's padding, the scrollbar the
    // fit addon reserves and the hairline come to 24 across and 6 down.
    expect(peekSize({ cols: 80, rows: 24, ...cell }, workspace)).toEqual({
      width: 672 + 24,
      height: 24 * 17 + 6 + PEEK_HEADER_PX,
    })
  })

  it('rounds a fractional grid up, never down to one column fewer', () => {
    expect(peekSize({ cols: 57, rows: 1, cellWidth: 8.4286, cellHeight: 17 }, workspace).width).toBe(481 + 24)
  })

  it('caps at 70% of the width and 80% of the height', () => {
    expect(peekSize({ cols: 200, rows: 60, ...cell }, workspace)).toEqual({ width: 896, height: 640 })
  })

  it('takes the height cap alone when no tile shows the session', () => {
    expect(peekSize({ cols: PEEK_FALLBACK_COLS, rows: null, ...cell }, workspace)).toEqual({ width: 840 + 24, height: 640 })
  })
})

describe('Peek', () => {
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

  afterEach(() => {
    resetSurfacesForTest()
    resetChordsForTest()
  })

  it('offers Send from the header without closing the peek', () => {
    render(<Peek />)

    fireEvent.click(screen.getByRole('button', { name: /^Send/ }))

    expect(mockState.openSendToSession).toHaveBeenCalledWith({ targetSessionKey: 'alice:alice-shell' })
    expect(mockState.closeFloatingModal).not.toHaveBeenCalled()
  })

  it('is a glance: a press outside closes it, Escape closes it, and its Close word does too', () => {
    render(<Peek />)
    const peek = screen.getByRole('dialog', { name: 'Peek alice-shell' })
    expect(screen.getByText('Loading terminal…')).toBeInTheDocument()

    fireEvent.pointerDown(peek)
    expect(mockState.closeFloatingModal).not.toHaveBeenCalled()

    fireEvent.pointerDown(document.body)
    expect(mockState.closeFloatingModal).toHaveBeenCalledTimes(1)

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(mockState.closeFloatingModal).toHaveBeenCalledTimes(2)

    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(mockState.closeFloatingModal).toHaveBeenCalledTimes(3)
  })

  it('gives a peek at an ended session the tile\'s note instead of a silent dead terminal', () => {
    mockState.sessions = []
    render(<Peek />)

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
    render(<Peek />)

    expect(screen.queryByText(/ended\. This frame shows its last output\./)).toBeNull()
    expect(screen.getByText('Loading terminal…')).toBeInTheDocument()
  })

  it('peeks through the same in-page terminal as a tile, on its own observer connection', () => {
    const { unmount } = render(<Peek />)

    expect(document.querySelector('.terminal-surface-host .terminal-surface')).not.toBeNull()
    expect(FakeSocket.latest().url).toContain('/terminal/ws?arg=peek&arg=alice-shell&arg=alice')

    unmount()

    expect(FakeSocket.latest().readyState).toBe(FakeSocket.CLOSED)
  })
})
