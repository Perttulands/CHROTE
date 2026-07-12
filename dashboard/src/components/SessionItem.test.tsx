import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import SessionItem from './SessionItem'
import { DEFAULT_SETTINGS } from '../types'

const mockState = vi.hoisted(() => ({
  assignedSessions: new Map<string, { workspaceId: string; windowId: string; windowIndex: number; colorIndex: number }>(),
  openFloatingModal: vi.fn(),
  openSendToSession: vi.fn(),
  handleSessionClick: vi.fn(),
  addSessionToWindow: vi.fn(),
  removeSessionFromWindow: vi.fn(),
  makeSessionPersistent: vi.fn(),
  makeSessionMortal: vi.fn(),
  dragListeners: { onPointerDown: vi.fn() },
  dragAttributes: { role: 'button', tabIndex: 0 },
  dragTransform: null as { x: number; y: number } | null,
  isDragging: false,
}))

vi.mock('@dnd-kit/core', () => ({
  useDraggable: () => ({
    attributes: mockState.dragAttributes,
    listeners: mockState.dragListeners,
    setNodeRef: vi.fn(),
    transform: mockState.dragTransform,
    isDragging: mockState.isDragging,
  }),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    assignedSessions: mockState.assignedSessions,
    handleSessionClick: mockState.handleSessionClick,
    deleteSession: vi.fn(),
    renameSession: vi.fn(),
    workspaces: {
      terminal1: { windows: [{ id: 'terminal1-window-0', colorIndex: 0 }], windowCount: 1 },
      terminal2: { windows: [], windowCount: 0 },
      terminal3: { windows: [], windowCount: 0 },
    },
    addSessionToWindow: mockState.addSessionToWindow,
    removeSessionFromWindow: mockState.removeSessionFromWindow,
    openFloatingModal: mockState.openFloatingModal,
    openSendToSession: mockState.openSendToSession,
    makeSessionPersistent: mockState.makeSessionPersistent,
    makeSessionMortal: mockState.makeSessionMortal,
    settings: {
      ...DEFAULT_SETTINGS,
      terminalUserColors: {
        alice: '#123456',
      },
    },
  }),
}))

vi.mock('./RoleBadge', () => ({
  default: () => null,
}))

describe('SessionItem user badge and context actions', () => {
  afterEach(() => {
    mockState.assignedSessions.clear()
    mockState.openFloatingModal.mockClear()
    mockState.openSendToSession.mockClear()
    mockState.handleSessionClick.mockClear()
    mockState.addSessionToWindow.mockClear()
    mockState.removeSessionFromWindow.mockClear()
    mockState.makeSessionPersistent.mockClear()
    mockState.makeSessionMortal.mockClear()
    mockState.dragListeners.onPointerDown.mockClear()
    mockState.dragTransform = null
    mockState.isDragging = false
    vi.restoreAllMocks()
  })

  it('renders a configured Unix user indicator from session.unixUser', () => {
    render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: false,
          group: 'main',
          unixUser: 'alice',
        }}
      />
    )

    const badge = screen.getByLabelText('Unix user alice')
    expect(badge).toHaveTextContent('A')
    expect(badge).toHaveAttribute('title', 'Unix user: alice')
    expect(badge).toHaveStyle({ backgroundColor: '#123456' })
  })

  it('places the Unix user badge before the attached terminal/window badge', () => {
    mockState.assignedSessions.set('alice-shell', {
      workspaceId: 'terminal1',
      windowId: 'window-1',
      windowIndex: 1,
      colorIndex: 0,
    })

    const { container } = render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: true,
          group: 'main',
          unixUser: 'alice',
        }}
      />
    )

    const rowText = Array.from(container.querySelector('.session-item')?.children ?? [])
      .map(child => child.textContent)

    expect(rowText.slice(1, 4)).toEqual(['A', 'T1 W1', 'alice-shell'])
  })

  it('offers peek and attach actions in the session context menu', () => {
    render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: false,
          group: 'main',
          unixUser: 'alice',
        }}
      />
    )

    fireEvent.contextMenu(screen.getByText('alice-shell'))
    fireEvent.click(screen.getByRole('button', { name: /Peek/i }))
    expect(mockState.openFloatingModal).toHaveBeenCalledWith('alice:alice-shell')

    fireEvent.contextMenu(screen.getByText('alice-shell'))
    expect(screen.getByText(/Attach to Window/i)).toBeInTheDocument()
    fireEvent.mouseEnter(screen.getByText(/Attach to Window/i))
    fireEvent.click(screen.getByRole('button', { name: /Window 1/i }))
    expect(mockState.addSessionToWindow).toHaveBeenCalledWith('terminal1', 'terminal1-window-0', 'alice-shell', 'alice')
  })

  it('offers Send to Session from context menu and ctrl-click shortcut', () => {
    render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: true,
          group: 'main',
          unixUser: 'alice',
        }}
      />
    )

    const row = screen.getByText('alice-shell')
    fireEvent.click(row, { ctrlKey: true })
    expect(mockState.openSendToSession).toHaveBeenCalledWith('alice:alice-shell')
    expect(mockState.handleSessionClick).not.toHaveBeenCalled()

    fireEvent.contextMenu(row)
    fireEvent.click(screen.getByRole('button', { name: /Send to Session/i }))
    expect(mockState.openSendToSession).toHaveBeenCalledTimes(2)
    expect(mockState.openSendToSession).toHaveBeenLastCalledWith('alice:alice-shell')
  })

  it('calls removeSessionFromWindow when Unassign is chosen from the context menu', () => {
    mockState.assignedSessions.set('alice:alice-shell', {
      workspaceId: 'terminal1',
      windowId: 'terminal1-window-0',
      windowIndex: 1,
      colorIndex: 0,
    })

    render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: true,
          group: 'main',
          unixUser: 'alice',
        }}
      />
    )

    fireEvent.contextMenu(screen.getByText('alice-shell'))
    fireEvent.click(screen.getByRole('button', { name: /Unassign/i }))

    expect(mockState.removeSessionFromWindow).toHaveBeenCalledWith(
      'terminal1',
      'terminal1-window-0',
      'alice:alice-shell',
    )
  })

  it('renders a lock indicator with identity for persistent sessions', () => {
    render(
      <SessionItem
        session={{
          name: 'codex-alpha',
          windows: 1,
          attached: false,
          group: 'codex',
          unixUser: 'alice',
          persistent: true,
          persistentIdentity: 'Maintains the VW Codex lane.',
          persistentAgentKind: 'codex',
        }}
      />
    )

    const lock = screen.getByLabelText('Persistent agent')
    expect(lock).toHaveTextContent('🔒')
    expect(lock).toHaveAttribute('title', 'Persistent codex agent: Maintains the VW Codex lane.')
  })

  it('offers make persistent for mortal sessions without asking for raw agent session id', async () => {
    vi.spyOn(window, 'prompt')
      .mockReturnValueOnce('Maintains the VW Codex lane.')
    mockState.makeSessionPersistent.mockResolvedValue(true)

    render(
      <SessionItem
        session={{
          name: 'codex-alpha',
          windows: 1,
          attached: false,
          group: 'codex',
          unixUser: 'alice',
        }}
      />
    )

    fireEvent.contextMenu(screen.getByText('codex-alpha'))
    fireEvent.click(screen.getByRole('button', { name: /Make persistent/i }))

    expect(mockState.makeSessionPersistent).toHaveBeenCalledWith('codex-alpha', {
      identity: 'Maintains the VW Codex lane.',
    }, 'alice')
    expect(window.prompt).toHaveBeenCalledTimes(1)
    expect(window.prompt).not.toHaveBeenCalledWith(expect.stringMatching(/session id/i), expect.anything())
  })

  it('offers make mortal for persistent sessions and protects direct kill', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    mockState.makeSessionMortal.mockResolvedValue(true)

    render(
      <SessionItem
        session={{
          name: 'codex-alpha',
          windows: 1,
          attached: false,
          group: 'codex',
          unixUser: 'alice',
          persistent: true,
          persistentIdentity: 'Maintains the VW Codex lane.',
          persistentAgentKind: 'codex',
        }}
      />
    )

    fireEvent.contextMenu(screen.getByText('codex-alpha'))
    expect(screen.queryByRole('button', { name: /Kill Session/i })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Make mortal/i }))

    expect(mockState.makeSessionMortal).toHaveBeenCalledWith('codex-alpha', 'alice')
  })

  it('uses a pointer-only non-interactive grip while keeping grip clicks inert and row clicks intact', () => {
    const { container } = render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: false,
          group: 'main',
          unixUser: 'alice',
        }}
      />
    )

    const row = container.querySelector('.session-item') as HTMLElement
    const handle = container.querySelector('.session-drag-handle') as HTMLElement

    expect(row).not.toHaveAttribute('role', 'button')
    expect(row).not.toHaveAttribute('tabindex', '0')
    expect(handle.tagName).toBe('SPAN')
    expect(handle).toHaveAttribute('aria-hidden', 'true')
    expect(handle).toHaveAttribute('title', 'Drag alice-shell (Unix user alice)')
    expect(handle).not.toHaveAttribute('role')
    expect(handle).not.toHaveAttribute('tabindex')
    expect(handle).not.toHaveAttribute('aria-roledescription')
    expect(handle).not.toHaveAttribute('aria-describedby')
    expect(handle).not.toHaveAttribute('aria-pressed')

    fireEvent.pointerDown(handle, { pointerType: 'mouse' })
    expect(mockState.dragListeners.onPointerDown).toHaveBeenCalledTimes(1)

    fireEvent.pointerDown(handle, { pointerType: 'touch' })
    expect(mockState.dragListeners.onPointerDown).toHaveBeenCalledTimes(2)

    fireEvent.click(handle)
    expect(mockState.handleSessionClick).not.toHaveBeenCalled()

    fireEvent.click(screen.getByText('alice-shell'))
    expect(mockState.handleSessionClick).toHaveBeenCalledWith('alice:alice-shell')
  })

  it('clears row long-press timers on move, end, cancel, and handle-origin touchstart while ordinary long-press still opens', () => {
    vi.useFakeTimers()
    try {
      const { container } = render(
        <SessionItem
          session={{
            name: 'alice-shell',
            windows: 1,
            attached: false,
            group: 'main',
            unixUser: 'alice',
          }}
        />
      )

      const row = container.querySelector('.session-item') as HTMLElement
      const handle = container.querySelector('.session-drag-handle') as HTMLElement
      const touch = { identifier: 1, target: handle, clientX: 32, clientY: 48 }
      const rowTouch = { ...touch, target: row }

      fireEvent.touchStart(row, { touches: [rowTouch], changedTouches: [rowTouch] })
      fireEvent.touchMove(row, { touches: [rowTouch], changedTouches: [rowTouch] })
      act(() => vi.advanceTimersByTime(600))
      expect(container.querySelector('.session-context-menu')).toBeNull()

      fireEvent.touchStart(row, { touches: [rowTouch], changedTouches: [rowTouch] })
      fireEvent.touchEnd(row, { touches: [], changedTouches: [rowTouch] })
      act(() => vi.advanceTimersByTime(600))
      expect(container.querySelector('.session-context-menu')).toBeNull()

      fireEvent.touchStart(row, { touches: [rowTouch], changedTouches: [rowTouch] })
      fireEvent.touchCancel(row, { touches: [], changedTouches: [rowTouch] })
      fireEvent.pointerDown(handle, { pointerType: 'touch' })
      act(() => vi.advanceTimersByTime(600))
      expect(container.querySelector('.session-context-menu')).toBeNull()

      fireEvent.touchStart(row, { touches: [rowTouch], changedTouches: [rowTouch] })
      fireEvent.touchStart(handle, { touches: [touch], changedTouches: [touch] })
      act(() => vi.advanceTimersByTime(600))
      expect(container.querySelector('.session-context-menu')).toBeNull()

      fireEvent.touchStart(row, { touches: [rowTouch], changedTouches: [rowTouch] })
      act(() => vi.advanceTimersByTime(600))
      expect(container.querySelector('.session-context-menu')).toBeInTheDocument()
    } finally {
      vi.clearAllTimers()
      vi.useRealTimers()
    }
  })

  it('clears the pending long-press timer on unmount before its state callback can run', () => {
    vi.useFakeTimers()
    try {
      const { container, unmount } = render(
        <SessionItem
          session={{
            name: 'alice-shell',
            windows: 1,
            attached: false,
            group: 'main',
            unixUser: 'alice',
          }}
        />
      )

      const row = container.querySelector('.session-item') as HTMLElement
      const touch = { identifier: 1, target: row, clientX: 32, clientY: 48 }
      fireEvent.touchStart(row, { touches: [touch], changedTouches: [touch] })
      expect(vi.getTimerCount()).toBe(1)

      unmount()
      expect(vi.getTimerCount()).toBe(0)
      act(() => vi.advanceTimersByTime(600))
      expect(vi.getTimerCount()).toBe(0)
    } finally {
      vi.clearAllTimers()
      vi.useRealTimers()
    }
  })

  it('keeps the mounted source stationary and invisible while the overlay moves', () => {
    mockState.dragTransform = { x: 48, y: 24 }
    mockState.isDragging = true

    const { container } = render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: false,
          group: 'main',
          unixUser: 'alice',
        }}
      />
    )

    const row = container.querySelector('.session-item') as HTMLElement
    expect(row).toHaveClass('dragging')
    expect(row.style.transform).toBe('')
    expect(row.style.transition).toBe('none')
    expect(row.style.opacity).toBe('0')
  })

})
