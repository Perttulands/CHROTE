import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import SessionItem from './SessionItem'
import { DEFAULT_SETTINGS, TERMINAL_WORKSPACE_IDS } from '../types'

const mockState = vi.hoisted(() => ({
  assignedSessions: new Map<string, { workspaceId: string; windowId: string; windowIndex: number; colorIndex: number }>(),
  openFloatingModal: vi.fn(),
  openSendToSession: vi.fn(),
  handleSessionClick: vi.fn(),
  focusSessionAssignment: vi.fn(),
  addSessionToWindow: vi.fn(),
  removeSessionFromWindow: vi.fn(),
  makeSessionPersistent: vi.fn(),
  makeSessionMortal: vi.fn(),
  deleteSession: vi.fn(),
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
    deleteSession: mockState.deleteSession,
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

describe('SessionItem user badge and context actions', () => {
  afterEach(() => {
    mockState.assignedSessions.clear()
    mockState.openFloatingModal.mockClear()
    mockState.openSendToSession.mockClear()
    mockState.handleSessionClick.mockClear()
    mockState.focusSessionAssignment.mockClear()
    mockState.addSessionToWindow.mockClear()
    mockState.removeSessionFromWindow.mockClear()
    mockState.makeSessionPersistent.mockClear()
    mockState.makeSessionMortal.mockClear()
    mockState.deleteSession.mockClear()
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
          persistentHealth: 'healthy',
          persistentIdentity: 'Maintains the VW Codex lane.',
          persistentAgentKind: 'codex',
        }}
      />
    )

    const lock = screen.getByLabelText('Persistent agent')
    expect(lock).toHaveTextContent('🔒')
    expect(lock).toHaveAttribute(
      'title',
      'Locked codex agent, supervised by systemd: Maintains the VW Codex lane.',
    )
    // A healthy lock shows the lock and nothing else: a badge that is always
    // present carries no information.
    expect(screen.queryByLabelText(/^Supervision:/)).toBeNull()
  })

  it.each([
    ['degraded', 'unconfirmed'],
    ['failed', 'failed'],
    ['inactive', 'stopped'],
  ] as const)('renders unit health %s exactly', (health, label) => {
    render(
      <SessionItem
        session={{
          name: `agent-${health}`,
          windows: 1,
          attached: false,
          group: 'codex',
          unixUser: 'alice',
          persistent: true,
          persistentHealth: health,
          persistentAgentKind: 'codex',
        }}
      />
    )

    expect(screen.getByLabelText(`Supervision: ${label}`)).toHaveTextContent(label)
  })

  it('names the unit and its trouble in the lock tooltip', () => {
    render(
      <SessionItem
        session={{
          name: 'hermes-scout',
          windows: 1,
          attached: false,
          group: 'hermes',
          unixUser: 'alice',
          persistent: true,
          persistentHealth: 'degraded',
          persistentUnit: 'chrote-agent@hermes-scout.service',
          persistentActiveState: 'active',
          persistentDetail: 'unit is running a different transcript than the one this lock configured',
          persistentAgentKind: 'hermes',
          persistentAgentSessionId: 'hermes-session-20260715T100000Z',
          persistentHermesProfile: 'scout',
        }}
      />
    )

    expect(screen.getByLabelText('Supervision: unconfirmed')).toHaveTextContent('unconfirmed')
    expect(screen.getByLabelText('Persistent agent')).toHaveAttribute(
      'title',
      'Locked hermes agent, supervised by systemd · Hermes profile scout · chrote-agent@hermes-scout.service · unit active · unit is running a different transcript than the one this lock configured',
    )
  })

  it('shows a bounded supervision error with its support reference', () => {
    render(
      <SessionItem
        session={{
          name: 'codex-alpha', windows: 1, attached: false, group: 'codex',
          unixUser: 'alice', persistent: true, persistentHealth: 'degraded',
          persistentDetail: 'unit status is unavailable',
          persistentDetailCode: 'unit-unreachable',
          persistentCorrelationId: 'pa-0123456789abcdef',
        }}
      />
    )

    expect(screen.getByLabelText('Persistent agent')).toHaveAttribute(
      'title',
      expect.stringContaining('reference pa-0123456789abcdef'),
    )
  })

  it('offers make persistent for mortal sessions without asking for raw agent session id', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
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

    expect(window.confirm).toHaveBeenCalledWith(expect.stringMatching(/restart this pane.*same native agent session/i))
    expect(mockState.makeSessionPersistent).toHaveBeenCalledWith('codex-alpha', {
      identity: 'Maintains the VW Codex lane.',
    }, 'alice')
    expect(window.prompt).toHaveBeenCalledTimes(1)
    expect(window.prompt).not.toHaveBeenCalledWith(expect.stringMatching(/session id/i), expect.anything())
  })

  it('does not offer locking when startup preflight marked the capability unavailable', () => {
    render(
      <SessionItem
        session={{
          name: 'codex-unavailable',
          windows: 1,
          attached: false,
          group: 'codex',
          unixUser: 'alice',
          persistentAvailable: false,
          persistentCapabilityDetail: 'agent unit template is not loaded for alice',
        }}
      />
    )

    fireEvent.contextMenu(screen.getByText('codex-unavailable'))
    expect(screen.queryByRole('button', { name: /^Make persistent$/i })).toBeNull()
    const unavailable = screen.getByRole('button', { name: /Persistence unavailable/i })
    expect(unavailable).toBeDisabled()
    expect(unavailable).toHaveAttribute('title', 'agent unit template is not loaded for alice')
  })

  it('surfaces available Hermes profile identity when making a session persistent', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    vi.spyOn(window, 'prompt')
      .mockReturnValueOnce('Keeps Hermes scout alive.')
    mockState.makeSessionPersistent.mockResolvedValue(true)

    render(
      <SessionItem
        session={{
          name: 'hermes-scout',
          windows: 1,
          attached: false,
          group: 'hermes',
          unixUser: 'alice',
          persistentAgentKind: 'hermes',
          persistentAgentSessionId: 'hermes-session-20260715T100000Z',
          persistentHermesProfile: 'scout',
        }}
      />
    )

    fireEvent.contextMenu(screen.getByText('hermes-scout'))
    fireEvent.click(screen.getByRole('button', { name: /Make persistent/i }))

    expect(window.prompt).toHaveBeenCalledWith(expect.stringContaining('Hermes profile scout'), '')
    expect(mockState.makeSessionPersistent).toHaveBeenCalledWith('hermes-scout', {
      identity: 'Keeps Hermes scout alive.',
      agentKind: 'hermes',
      agentSessionId: 'hermes-session-20260715T100000Z',
    }, 'alice')
  })

  it('does not lock when the operator declines the required pane restart', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    vi.spyOn(window, 'prompt')

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

    expect(window.prompt).not.toHaveBeenCalled()
    expect(mockState.makeSessionPersistent).not.toHaveBeenCalled()
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
    // Kill is no longer hidden for a locked session; it is offered as the honest
    // two-step, because killing without unlocking would be undone by the unit.
    expect(screen.getByRole('button', { name: /Stop supervision and kill/i })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Make mortal \(metadata only\)/i }))

    expect(mockState.makeSessionMortal).toHaveBeenCalledWith('codex-alpha', 'alice')
    expect(window.confirm).toHaveBeenCalledWith(
      expect.stringContaining('no longer be restarted after a crash or reboot'),
    )
  })

  it('aborts locked-session deletion when stopping supervision fails', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    mockState.makeSessionMortal.mockResolvedValue(false)

    render(
      <SessionItem
        session={{
          name: 'codex-alpha', windows: 1, attached: false, group: 'codex',
          unixUser: 'alice', persistent: true,
        }}
      />
    )

    fireEvent.contextMenu(screen.getByText('codex-alpha'))
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Stop supervision and kill/i }))
    })

    expect(mockState.makeSessionMortal).toHaveBeenCalledWith('codex-alpha', 'alice')
    expect(mockState.deleteSession).not.toHaveBeenCalled()
  })

  it('keeps a failed unlock visible and retryable', () => {
    render(
      <SessionItem
        session={{
          name: 'codex-alpha', windows: 1, attached: false, group: 'codex',
          unixUser: 'alice', persistent: true,
          persistentUnlockFailed: true,
          persistentUnlockError: 'target user bus is unavailable',
        }}
      />
    )

    expect(screen.getByLabelText('Supervision: unlock failed')).toHaveAttribute(
      'title',
      expect.stringContaining('target user bus is unavailable'),
    )
    fireEvent.contextMenu(screen.getByText('codex-alpha'))
    expect(screen.getByRole('button', { name: /Make mortal \(metadata only\)/i })).toBeInTheDocument()
  })

  it('renders an absent locked session as status-only with an unlock action', () => {
    render(
      <SessionItem
        session={{
          name: 'codex-alpha', windows: 0, attached: false, group: 'codex',
          unixUser: 'alice', persistent: true, persistentSessionMissing: true,
          persistentHealth: 'failed', persistentUnit: 'chrote-agent@codex-alpha.service',
          persistentDetail: 'unit failed; see the agent unit journal',
        }}
      />
    )

    fireEvent.click(screen.getByText('codex-alpha'))
    expect(mockState.handleSessionClick).not.toHaveBeenCalled()

    fireEvent.contextMenu(screen.getByText('codex-alpha'))
    expect(screen.getByRole('button', { name: /Make mortal \(metadata only\)/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Peek' })).toBeNull()
    expect(screen.queryByRole('button', { name: /Stop supervision and kill/i })).toBeNull()
    expect(screen.getByLabelText('Persistent agent')).toHaveAttribute(
      'title',
      expect.stringContaining('tmux session absent'),
    )
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
