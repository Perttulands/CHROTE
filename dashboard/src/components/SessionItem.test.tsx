import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import SessionItem from './SessionItem'
import type { TmuxSession } from '../types'
import { DEFAULT_SETTINGS, TERMINAL_WORKSPACE_IDS } from '../types'

const mockState = vi.hoisted(() => ({
  assignedSessions: new Map<string, { workspaceId: string; windowId: string; windowIndex: number; colorIndex: number }>(),
  openFloatingModal: vi.fn(),
  openSendToSession: vi.fn(),
  handleSessionClick: vi.fn(),
  focusSessionAssignment: vi.fn(),
  addSessionToWindow: vi.fn(),
  removeSessionFromWindow: vi.fn(),
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
    focusSessionAssignment: mockState.focusSessionAssignment,
    deleteSession: vi.fn(),
    renameSession: vi.fn(),
    workspaces: {
      terminal1: { windows: [{ id: 'terminal1-window-0', colorIndex: 0 }], windowCount: 1 },
      terminal2: { windows: [], windowCount: 0 },
      terminal3: { windows: [], windowCount: 0 },
    },
    workspaceIds: TERMINAL_WORKSPACE_IDS,
    addSessionToWindow: mockState.addSessionToWindow,
    removeSessionFromWindow: mockState.removeSessionFromWindow,
    openFloatingModal: mockState.openFloatingModal,
    openSendToSession: mockState.openSendToSession,
    settings: {
      ...DEFAULT_SETTINGS,
      terminalUserColors: {
        alice: '#123456',
      },
    },
  }),
}))

describe('SessionItem user badge and context actions', () => {
  afterEach(() => {
    mockState.assignedSessions.clear()
    mockState.openFloatingModal.mockClear()
    mockState.openSendToSession.mockClear()
    mockState.handleSessionClick.mockClear()
    mockState.focusSessionAssignment.mockClear()
    mockState.addSessionToWindow.mockClear()
    mockState.removeSessionFromWindow.mockClear()
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

  it('shows shell-only foreground state separately from tmux attachment', () => {
    render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: true,
          group: 'main',
          unixUser: 'alice',
          currentCommand: 'bash',
        }}
      />
    )

    expect(screen.getByTitle('Foreground process reported by tmux: bash')).toHaveTextContent('shell')
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

    expect(rowText.slice(0, 3)).toEqual(['A', 'T1 W1', 'alice-shell'])

    fireEvent.click(screen.getByRole('button', { name: 'Focus assigned window T1 W1' }))
    expect(mockState.focusSessionAssignment).toHaveBeenCalledWith('alice-shell')
    expect(mockState.handleSessionClick).not.toHaveBeenCalled()
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

    fireEvent.click(screen.getByRole('button', { name: 'Session actions for alice-shell' }))
    fireEvent.click(screen.getByRole('button', { name: /Peek/i }))
    expect(mockState.openFloatingModal).toHaveBeenCalledWith('alice:alice-shell')

    fireEvent.click(screen.getByRole('button', { name: 'Session actions for alice-shell' }))
    expect(screen.getByText(/Attach to Window/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Attach to Window/i }))
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
    expect(mockState.openSendToSession).not.toHaveBeenCalled()
    expect(mockState.handleSessionClick).toHaveBeenCalledWith('alice:alice-shell')

    fireEvent.contextMenu(row)
    fireEvent.click(screen.getByRole('button', { name: /Send to Session/i }))
    expect(mockState.openSendToSession).toHaveBeenCalledTimes(1)
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

  it('treats legacy persistence metadata as an ordinary session', () => {
    render(
      <SessionItem
        session={{
          name: 'codex-alpha',
          windows: 1,
          attached: false,
          group: 'codex',
          unixUser: 'alice',
          persistent: true,
          persistentHealth: 'failed',
        } as TmuxSession & { persistent: boolean; persistentHealth: string }}
      />
    )

    expect(screen.queryByLabelText('Persistent agent')).toBeNull()
    expect(screen.queryByLabelText(/^Supervision:/)).toBeNull()

    fireEvent.contextMenu(screen.getByText('codex-alpha'))
    expect(screen.queryByRole('button', { name: /Make persistent|Make mortal|supervision/i })).toBeNull()
    expect(screen.getByRole('button', { name: /Rename/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Peek/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Send to Session/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Attach to Window/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Kill Session/ })).toBeInTheDocument()
  })

  it('uses the whole session row as the drag surface without rendering a drag grip', () => {
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
    const menuButton = screen.getByRole('button', { name: 'Session actions for alice-shell' })

    expect(row).not.toHaveAttribute('role', 'button')
    expect(row).not.toHaveAttribute('tabindex', '0')
    expect(row).toHaveAttribute('title', 'Drag alice-shell (Unix user alice)')
    expect(container.querySelector('.session-drag-handle')).toBeNull()

    fireEvent.pointerDown(screen.getByText('alice-shell'), { pointerType: 'mouse' })
    expect(mockState.dragListeners.onPointerDown).toHaveBeenCalledTimes(1)

    fireEvent.pointerDown(menuButton, { pointerType: 'mouse' })
    expect(mockState.dragListeners.onPointerDown).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByText('alice-shell'))
    expect(mockState.handleSessionClick).toHaveBeenCalledWith('alice:alice-shell')
  })

  it('clears row long-press timers on move, end, cancel, and drag start while ordinary long-press still opens', () => {
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
      const rowTouch = { identifier: 1, target: row, clientX: 32, clientY: 48 }

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
      fireEvent.pointerDown(row, { pointerType: 'touch' })
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

  it('cancels the pending touch pointer sensor when long-press opens session actions', () => {
    vi.useFakeTimers()
    const pointerCancels: Event[] = []
    const recordPointerCancel = (event: Event) => pointerCancels.push(event)
    document.addEventListener('pointercancel', recordPointerCancel)
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
      const touch = { identifier: 1, target: row, clientX: 32, clientY: 48 }
      fireEvent.pointerDown(row, {
        pointerId: 41,
        pointerType: 'touch',
        isPrimary: true,
        button: 0,
        buttons: 1,
      })
      fireEvent.touchStart(row, { touches: [touch], changedTouches: [touch] })

      act(() => vi.advanceTimersByTime(500))

      expect(container.querySelector('.session-context-menu')).toBeInTheDocument()
      expect(pointerCancels).toHaveLength(1)
      expect((pointerCancels[0] as PointerEvent).pointerId).toBe(41)
      expect((pointerCancels[0] as PointerEvent).pointerType).toBe('touch')
    } finally {
      document.removeEventListener('pointercancel', recordPointerCancel)
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
