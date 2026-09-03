import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import SessionItem from './SessionItem'
import type { TmuxSession } from '../types'
import { DEFAULT_SETTINGS, TERMINAL_WORKSPACE_IDS } from '../types'
import { DEFAULT_THEME } from '../theme/theme'

const mockState = vi.hoisted(() => ({
  assignedSessions: new Map<string, { workspaceId: string; windowId: string; windowIndex: number; colorIndex: number }>(),
  openFloatingModal: vi.fn(),
  openSendToSession: vi.fn(),
  handleSessionClick: vi.fn(),
  focusedWindowKey: null as string | null,
  addSessionToWindow: vi.fn(),
  removeSessionFromWindow: vi.fn(),
  renameSession: vi.fn(),
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
    focusedWindowKey: mockState.focusedWindowKey,
    deleteSession: vi.fn(),
    workspaces: {
      terminal1: { windows: [{ id: 'terminal1-window-0', colorIndex: 0, activeSession: 'alice:alice-shell' }], windowCount: 1 },
      terminal2: { windows: [], windowCount: 0 },
      terminal3: { windows: [], windowCount: 0 },
    },
    workspaceIds: TERMINAL_WORKSPACE_IDS,
    addSessionToWindow: mockState.addSessionToWindow,
    removeSessionFromWindow: mockState.removeSessionFromWindow,
    renameSession: mockState.renameSession,
    openFloatingModal: mockState.openFloatingModal,
    openSendToSession: mockState.openSendToSession,
    settings: DEFAULT_SETTINGS,
    terminalUsers: ['alice', 'bob'],
  }),
}))

// A row draws its name as head and tail spans, so the label is found by the
// full name it carries in its title rather than as one text node.
const rowLabel = (name: string) => screen.getByTitle(name)

describe('SessionItem user badge and context actions', () => {
  afterEach(() => {
    mockState.assignedSessions.clear()
    mockState.openFloatingModal.mockClear()
    mockState.openSendToSession.mockClear()
    mockState.handleSessionClick.mockClear()
    mockState.focusedWindowKey = null
    mockState.addSessionToWindow.mockClear()
    mockState.removeSessionFromWindow.mockClear()
    mockState.renameSession.mockClear()
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
    // alice is the server's first terminal user, so she wears the first identity colour.
    expect(badge).toHaveStyle({ backgroundColor: DEFAULT_THEME.identity[0] })
  })

  // What runs in a row is a mark: the agent's own, nothing at all for a shell,
  // and the bare command for anything else.
  it('marks the harness running in the session and says nothing for a shell', () => {
    const shell = { name: 'alice-shell', windows: 1, attached: true, group: 'main', unixUser: 'alice', currentCommand: 'bash' }
    const { container, rerender } = render(<SessionItem session={shell} />)

    expect(container.querySelector('.harness-mark, .harness-command')).toBeNull()
    expect(container.textContent).not.toContain('bash')

    rerender(<SessionItem session={{ ...shell, currentCommand: 'claude' }} />)
    expect(container.querySelector('[data-harness="claude-code"]')).not.toBeNull()
    expect(screen.getByTitle('tmux reports claude')).toBeInTheDocument()

    rerender(<SessionItem session={{ ...shell, currentCommand: 'codex' }} />)
    expect(container.querySelector('[data-harness="codex"]')).not.toBeNull()

    rerender(<SessionItem session={{ ...shell, currentCommand: 'sleep' }} />)
    expect(container.querySelector('.harness-command')).toHaveTextContent('sleep')
    expect(container.querySelector('[data-harness]')).toBeNull()
  })

  // Prefixes are shared; the tail is what tells two sessions apart.
  it('keeps the tail of a hyphenated name and clips only its head', () => {
    const { container } = render(
      <SessionItem session={{ name: 'claude-chrote-fable', windows: 1, attached: false, group: 'main', unixUser: 'alice' }} />
    )

    const label = container.querySelector('.session-label') as HTMLElement
    expect(label).toHaveAttribute('title', 'claude-chrote-fable')
    expect(label.querySelector('.session-label-head')).toHaveTextContent('claude-chrote-')
    expect(label.querySelector('.session-label-tail')).toHaveTextContent('fable')
  })

  it('marks a session whose facts contradict its appearance, with the fact on hover', () => {
    render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 2,
          attached: true,
          group: 'main',
          unixUser: 'alice',
          panes: 1,
          width: 100,
          height: 30,
          sizePinned: true,
          mouseEnabled: false,
          foreignClients: ['/dev/pts/12'],
        }}
      />
    )

    const pinned = screen.getByLabelText(/^Fixed size:/)
    expect(pinned).toHaveTextContent('⊡')
    expect(pinned).toHaveAttribute('title', expect.stringContaining('100x30'))
    expect(screen.getByLabelText(/^Foreign client attached:/)).toHaveAttribute(
      'title',
      expect.stringContaining('/dev/pts/12'),
    )
    expect(screen.getByLabelText(/^More than one window or pane:/)).toBeInTheDocument()
    expect(screen.getByLabelText(/^Mouse off:/)).toBeInTheDocument()
  })

  it('marks nothing on a session that is what it looks like', () => {
    const { container } = render(
      <SessionItem
        session={{
          name: 'alice-shell',
          windows: 1,
          attached: true,
          group: 'main',
          unixUser: 'alice',
          panes: 1,
          width: 120,
          height: 40,
          mouseEnabled: true,
        }}
      />
    )

    expect(container.querySelectorAll('.session-badge')).toHaveLength(0)
  })

  it('cancels rename with Escape and rejects an empty replacement', () => {
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

    fireEvent.contextMenu(rowLabel('alice-shell'))
    fireEvent.click(screen.getByRole('menuitem', { name: /Rename/ }))
    const cancelled = screen.getByRole('textbox')
    fireEvent.change(cancelled, { target: { value: 'discarded' } })
    fireEvent.keyDown(cancelled, { key: 'Escape' })
    expect(mockState.renameSession).not.toHaveBeenCalled()
    expect(rowLabel('alice-shell')).toBeInTheDocument()

    fireEvent.contextMenu(rowLabel('alice-shell'))
    fireEvent.click(screen.getByRole('menuitem', { name: /Rename/ }))
    const empty = screen.getByRole('textbox')
    fireEvent.change(empty, { target: { value: '' } })
    fireEvent.keyDown(empty, { key: 'Enter' })
    expect(mockState.renameSession).not.toHaveBeenCalled()
    expect(rowLabel('alice-shell')).toBeInTheDocument()
  })

  it('opens the row menu without a location chip in front of the name', () => {
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

    // Badge, then mark, then the name: no chip repeats the tile's address.
    expect(rowText.slice(0, 2)).toEqual(['A', 'alice-shell'])
    expect(container.querySelector('.window-location-chip')).toBeNull()
    expect(screen.queryByRole('button', { name: /Focus assigned window/ })).not.toBeInTheDocument()
  })

  // Where the operator is typing, said once: the row of the session the focused
  // tile is showing carries the mark, and every other row stays plain.
  it('marks the row whose session the focused tile is showing', () => {
    const session: TmuxSession = { name: 'alice-shell', windows: 1, attached: true, group: 'main', unixUser: 'alice' }
    const { container, rerender } = render(<SessionItem session={session} />)

    expect(container.querySelector('.session-item')).not.toHaveClass('in-focused-tile')

    mockState.focusedWindowKey = 'terminal1-terminal1-window-0'
    rerender(<SessionItem session={session} />)
    expect(container.querySelector('.session-item')).toHaveClass('in-focused-tile')
    expect(container.querySelector('.session-item')).toHaveAttribute('aria-current', 'true')

    rerender(<SessionItem session={{ ...session, name: 'other-shell' }} />)
    expect(container.querySelector('.session-item')).not.toHaveClass('in-focused-tile')
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
    expect(screen.queryByRole('menuitem', { name: /Unassign/i })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('menuitem', { name: /Peek/i }))
    expect(mockState.openFloatingModal).toHaveBeenCalledWith('alice:alice-shell')

    fireEvent.click(screen.getByRole('button', { name: 'Session actions for alice-shell' }))
    expect(screen.getByText(/Attach to window/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('menuitem', { name: /Attach to window/i }))
    fireEvent.click(screen.getByRole('menuitem', { name: /Window 1/i }))
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

    const row = rowLabel('alice-shell')
    fireEvent.click(row, { ctrlKey: true })
    expect(mockState.openSendToSession).not.toHaveBeenCalled()
    expect(mockState.handleSessionClick).toHaveBeenCalledWith('alice:alice-shell')

    fireEvent.contextMenu(row)
    fireEvent.click(screen.getByRole('menuitem', { name: /Send to session/i }))
    expect(mockState.openSendToSession).toHaveBeenCalledTimes(1)
    expect(mockState.openSendToSession).toHaveBeenLastCalledWith({ targetSessionKey: 'alice:alice-shell' })
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

    fireEvent.contextMenu(rowLabel('alice-shell'))
    fireEvent.click(screen.getByRole('menuitem', { name: /Unassign/i }))

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

    fireEvent.contextMenu(rowLabel('codex-alpha'))
    expect(screen.queryByRole('menuitem', { name: /Make persistent|Make mortal|supervision/i })).toBeNull()
    expect(screen.getByRole('menuitem', { name: /Rename/ })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Peek/ })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Send to session/ })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Attach to window/ })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Kill session/ })).toBeInTheDocument()
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

    fireEvent.pointerDown(rowLabel('alice-shell'), { pointerType: 'mouse' })
    expect(mockState.dragListeners.onPointerDown).toHaveBeenCalledTimes(1)

    fireEvent.pointerDown(menuButton, { pointerType: 'mouse' })
    expect(mockState.dragListeners.onPointerDown).toHaveBeenCalledTimes(1)

    fireEvent.click(rowLabel('alice-shell'))
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
      expect(document.querySelector('.menu-sheet')).toBeNull()

      fireEvent.touchStart(row, { touches: [rowTouch], changedTouches: [rowTouch] })
      fireEvent.touchEnd(row, { touches: [], changedTouches: [rowTouch] })
      act(() => vi.advanceTimersByTime(600))
      expect(document.querySelector('.menu-sheet')).toBeNull()

      fireEvent.touchStart(row, { touches: [rowTouch], changedTouches: [rowTouch] })
      fireEvent.touchCancel(row, { touches: [], changedTouches: [rowTouch] })
      fireEvent.pointerDown(row, { pointerType: 'touch' })
      act(() => vi.advanceTimersByTime(600))
      expect(document.querySelector('.menu-sheet')).toBeNull()

      fireEvent.touchStart(row, { touches: [rowTouch], changedTouches: [rowTouch] })
      act(() => vi.advanceTimersByTime(600))
      expect(document.querySelector('.menu-sheet')).toBeInTheDocument()
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

      expect(document.querySelector('.menu-sheet')).toBeInTheDocument()
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
