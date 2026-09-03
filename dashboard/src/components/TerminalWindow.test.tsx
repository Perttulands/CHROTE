import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalWindow from './TerminalWindow'
import { DEFAULT_SETTINGS } from '../types'
import { sessionEvidenceFrom } from '../terminal/tileState'
import { loadStoredState } from '../context/workspaceLayouts'

const refreshSessions = vi.fn()
const createSession = vi.fn()
const addSessionToWindow = vi.fn()
const announce = vi.fn()
const removeSessionFromWindow = vi.fn()
const renameSession = vi.fn()
const deleteSession = vi.fn()
const reconnect = vi.fn()
const claim = vi.fn()
const redialIfDropped = vi.fn()
const fit = vi.fn()
const setFocusedWindowKey = vi.fn()
const setActiveSession = vi.fn()
const openSendToSession = vi.fn()
const restartSession = vi.fn()
const draggableState = vi.hoisted(() => ({
  transform: null as { x: number, y: number } | null,
  isDragging: false,
  listeners: { onPointerDown: vi.fn() },
}))
const droppableState = vi.hoisted(() => ({
  isOver: false,
  active: null as { data: { current: Record<string, unknown> } } | null,
}))
const poolState = vi.hoisted(() => ({ connectionStates: new Map<string, string>() }))
const mockSessions = vi.hoisted(() => ([
  { name: 'forge-existing', windows: 1, attached: false, group: 'forge', unixUser: 'build', cwd: '/srv/forge', currentCommand: 'codex' },
  { name: 'shell-existing', windows: 1, attached: false, group: 'shell', unixUser: 'alice', cwd: '/srv/live-shell', currentCommand: 'bash' },
  { name: 'shared-existing', windows: 1, attached: false, group: 'shell', unixUser: 'alice', currentCommand: 'codex' },
  { name: 'shared-existing', windows: 1, attached: false, group: 'shell', unixUser: 'build', currentCommand: 'bash' },
  { name: 'pinned-existing', windows: 1, attached: false, group: 'shell', unixUser: 'alice', cwd: '/srv/pinned', currentCommand: 'bash', sizePinned: true, width: 100, height: 30 },
  { name: 'ssh-held', windows: 1, attached: true, group: 'shell', unixUser: 'alice', currentCommand: 'bash', foreignClients: ['/dev/pts/12'] },
  { name: 'napping-existing', windows: 1, attached: false, group: 'shell', unixUser: 'alice', currentCommand: 'sleep' },
]))
const pooledTerminals = new Map<string, { reconnect: () => void; claim: () => void; redialIfDropped: () => void; fit: () => void; focus: () => void }>()

vi.mock('@dnd-kit/core', () => ({
  useDraggable: () => ({
    attributes: { role: 'button', tabIndex: 0 },
    listeners: draggableState.listeners,
    setNodeRef: vi.fn(),
    transform: draggableState.transform,
    isDragging: draggableState.isDragging,
  }),
  useDroppable: () => ({ setNodeRef: vi.fn(), isOver: droppableState.isOver, active: droppableState.active }),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: {
      ...DEFAULT_SETTINGS,
      terminalLaunchUsers: {
        ...DEFAULT_SETTINGS.terminalLaunchUsers,
        terminal3: 'build',
      },
      terminalSessionPrefixes: {
        ...DEFAULT_SETTINGS.terminalSessionPrefixes,
        build: 'forge',
      },
    },
    terminalUsers: ['alice', 'build'],
    sessions: mockSessions,
    sessionEvidence: sessionEvidenceFrom({
      sessions: mockSessions,
      loading: false,
      error: null,
      partialAnsweringUsers: null,
    }),
    layoutPresets: [{ id: 'preset-1', name: 'Focus Layout', createdAt: 1, workspaces: {} }],
    refreshSessions,
    createSession,
    addSessionToWindow,
    removeSessionFromWindow,
    setActiveSession,
    setWindowCount: vi.fn(),
    restartSession,
    loading: false,
    error: null,
    partialAnsweringUsers: null,
    focusedWindowKey: null,
    setFocusedWindowKey,
    openSendToSession,
    saveCurrentLayout: vi.fn(),
    loadPreset: vi.fn(),
    deleteSession,
    renameSession,
  }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce }),
}))

vi.mock('./TerminalPool', () => ({
  useTerminalPool: () => ({
    terminals: {
      get(sessionKey: string) {
        if (!pooledTerminals.has(sessionKey)) {
          pooledTerminals.set(sessionKey, {
            reconnect: () => reconnect(sessionKey),
            claim: () => claim(sessionKey),
            redialIfDropped: () => redialIfDropped(sessionKey),
            fit: () => fit(sessionKey),
            focus: vi.fn(),
          })
        }
        return pooledTerminals.get(sessionKey)
      },
    },
    connectionStates: poolState.connectionStates,
  }),
}))

vi.mock('./TerminalSurface', () => ({
  default: () => <div className="terminal-surface-host" />,
}))


// A tag draws its name as head and tail spans, so the label is found by the
// full name it carries in its title rather than as one text node.
const tagLabel = (name: string) => screen.getByTitle(name)

function dispatchContextMenu(target: Element) {
  const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 350, clientY: 230 })
  act(() => target.dispatchEvent(event))
  return event
}

describe('TerminalWindow launch user', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    createSession.mockResolvedValue('forge1')
    deleteSession.mockResolvedValue(true)
    renameSession.mockResolvedValue(true)
    draggableState.transform = null
    draggableState.isDragging = false
    draggableState.listeners.onPointerDown.mockClear()
    droppableState.isOver = false
    droppableState.active = null
    poolState.connectionStates = new Map()
    vi.stubGlobal('fetch', vi.fn((input: unknown) => (
      String(input).includes('/api/launch')
        ? Promise.resolve({
          ok: true,
          json: async () => ({
            harnesses: [{ id: 'claude-code', label: 'Claude Code' }, { id: 'shell', label: 'Shell' }],
            folders: ['/srv/chrote', '~'],
          }),
        })
        : Promise.resolve({ ok: true })
    )) as any)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
  })

  it('launches the configured harness in the configured folder and binds it to this window', async () => {
    let finishCreate!: (sessionName: string) => void
    createSession.mockReturnValue(new Promise(resolve => { finishCreate = resolve }))
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    // The empty window is the launcher, not a button that has to be pressed
    // before the operator learns what a new session would be.
    expect(container.querySelector('.tile-action-btn')).toBeNull()
    const launchButton = await screen.findByRole('button', { name: 'Launch claude in chrote' })
    expect(screen.getByLabelText('Session name')).toHaveValue('claude-chrote')

    fireEvent.click(launchButton)

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(launchButton).toBeDisabled()
    fireEvent.click(launchButton)
    expect(createSession).toHaveBeenCalledTimes(1)
    expect(createSession).toHaveBeenCalledWith({
      name: 'claude-chrote',
      unixUser: 'build',
      cwd: '/srv/chrote',
      harness: 'claude-code',
      workspaceId: 'terminal3',
      attachTo: { workspaceId: 'terminal3', windowId: 'terminal3-window-0' },
    })
    await act(async () => finishCreate('claude-chrote'))
    await waitFor(() => expect(launchButton).toBeEnabled())
  })

  // The tags are the tile's tabs; the arrows that stepped through them are
  // gone, and nothing in the header cycles a session any more.
  // Only a session tag carries a menu. Everywhere else in the tile the
  // browser's own right-click still belongs to the operator.
  it('leaves right-click to the browser everywhere but a session tag', async () => {
    const { container, rerender } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    const launchButton = await screen.findByRole('button', { name: 'Launch claude in chrote' })
    const launcherEvent = dispatchContextMenu(launchButton)

    expect(launcherEvent.defaultPrevented).toBe(false)
    expect(document.querySelector('.menu-sheet')).toBeNull()
    fireEvent.click(launchButton)
    await waitFor(() => expect(createSession).toHaveBeenCalled())

    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )
    const headerEvent = dispatchContextMenu(container.querySelector('.terminal-window-header') as HTMLElement)

    expect(headerEvent.defaultPrevented).toBe(false)
    expect(document.querySelector('.menu-sheet')).toBeNull()
  })

  it('opens only the requested actions for the clicked session tag', () => {
    const openFilesAtPath = vi.fn()
    const { rerender } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{
          id: 'terminal3-window-0',
          boundSessions: ['build:forge-existing', 'alice:shell-existing'],
          activeSession: 'build:forge-existing',
          colorIndex: 0,
        }}
        onOpenFilesAtPath={openFilesAtPath}
      />
    )

    const openInactiveMenu = () => dispatchContextMenu(tagLabel('shell-existing'))

    const sendEvent = openInactiveMenu()
    expect(sendEvent.defaultPrevented).toBe(true)
    const menuButtons = screen.getAllByRole('menuitem')
    expect(menuButtons).toHaveLength(8)
    for (const label of [
      'Send to session',
      'What this agent sees',
      'Reconnect frame',
      /Claim session/,
      'Refit frame',
      'Open files in working directory',
      'Rename session',
      'Kill session',
    ]) {
      expect(screen.getByRole('menuitem', { name: label })).toBeInTheDocument()
    }
    fireEvent.click(screen.getByRole('menuitem', { name: 'Send to session' }))
    expect(openSendToSession).toHaveBeenCalledWith({ targetSessionKey: 'alice:shell-existing' })

    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Reconnect frame' }))
    expect(reconnect).toHaveBeenCalledWith('alice:shell-existing')

    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Refit frame' }))
    expect(fit).toHaveBeenCalledWith('alice:shell-existing')

    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Open files in working directory' }))
    expect(openFilesAtPath).toHaveBeenCalledWith('/srv/live-shell')

    // Kill confirms in place: the first press arms the row, the second runs it.
    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Kill session' }))
    expect(deleteSession).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Confirm kill' }))
    expect(deleteSession).toHaveBeenCalledWith('shell-existing', 'alice')

    expect(setActiveSession).not.toHaveBeenCalled()

    // A session tmux reports no working directory for has nowhere to route to,
    // so the row is offered and refused rather than hidden.
    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['missing-session'], activeSession: 'missing-session', colorIndex: 0 }}
        onOpenFilesAtPath={openFilesAtPath}
      />
    )
    dispatchContextMenu(tagLabel('missing-session'))
    expect(screen.getByRole('menuitem', { name: 'Open files in working directory' })).toBeDisabled()
  })

  it('offers the sizing action as disabled with its reason on a session tmux has pinned', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{
          id: 'terminal3-window-0',
          boundSessions: ['alice:pinned-existing', 'alice:shell-existing'],
          activeSession: 'alice:pinned-existing',
          colorIndex: 0,
        }}
      />
    )

    dispatchContextMenu(tagLabel('pinned-existing'))
    const pinnedRefit = screen.getByRole('menuitem', { name: /^Refit frame/ })
    expect(pinnedRefit).toBeDisabled()
    expect(pinnedRefit).toHaveTextContent('Pinned at 100x30. tmux window-size is manual on this window, so CHROTE cannot resize it.')
    expect(pinnedRefit).toHaveAttribute('title', 'Pinned at 100x30. tmux window-size is manual on this window, so CHROTE cannot resize it.')
    fireEvent.click(pinnedRefit)
    expect(fit).not.toHaveBeenCalled()
    fireEvent.keyDown(document, { key: 'Escape' })

    dispatchContextMenu(tagLabel('shell-existing'))
    const ordinaryRefit = screen.getByRole('menuitem', { name: 'Refit frame' })
    expect(ordinaryRefit).toBeEnabled()
    expect(ordinaryRefit).not.toHaveAttribute('title')
    fireEvent.click(ordinaryRefit)
    expect(fit).toHaveBeenCalledWith('alice:shell-existing')
  })

  it('renames the exact qualified session from its attached tag menu and cancels with Escape', async () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{
          id: 'terminal3-window-0',
          boundSessions: ['build:forge-existing', 'alice:shell-existing'],
          activeSession: 'build:forge-existing',
          colorIndex: 0,
        }}
      />
    )

    const openMenu = () => dispatchContextMenu(tagLabel('shell-existing'))

    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Rename session' }))
    const renameInput = screen.getByRole('textbox', { name: 'Rename session shell-existing' })
    expect(renameInput).toHaveFocus()
    expect(renameInput).toHaveValue('shell-existing')
    fireEvent.change(renameInput, { target: { value: 'shell-renamed' } })
    fireEvent.keyDown(renameInput, { key: 'Enter' })

    await waitFor(() => expect(renameSession).toHaveBeenCalledWith('shell-existing', 'shell-renamed', 'alice'))
    expect(renameSession).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(screen.queryByRole('textbox', { name: 'Rename session shell-existing' })).not.toBeInTheDocument())

    renameSession.mockClear()
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Rename session' }))
    const cancelledInput = screen.getByRole('textbox', { name: 'Rename session shell-existing' })
    fireEvent.change(cancelledInput, { target: { value: 'discard-this' } })
    fireEvent.keyDown(cancelledInput, { key: 'Escape' })

    expect(renameSession).not.toHaveBeenCalled()
    expect(screen.queryByRole('textbox', { name: 'Rename session shell-existing' })).not.toBeInTheDocument()
    expect(tagLabel('shell-existing')).toBeInTheDocument()
  })

  it('uses the original bound key for terminal actions on supported bare legacy tags', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{
          id: 'terminal3-window-0',
          boundSessions: ['shell-existing'],
          activeSession: 'shell-existing',
          colorIndex: 0,
        }}
      />,
    )

    dispatchContextMenu(tagLabel('shell-existing'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Reconnect frame' }))
    expect(reconnect).toHaveBeenCalledWith('shell-existing')

    dispatchContextMenu(tagLabel('shell-existing'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Refit frame' }))
    expect(fit).toHaveBeenCalledWith('shell-existing')
  })

  it('opens the tag menu from the keyboard, and only from the tag itself', () => {
    const props = {
      workspaceId: 'terminal3' as const,
      window: { id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 },
    }
    const { container, rerender } = render(<TerminalWindow {...props} {...({ workspaceActive: true } as any)} />)
    const tag = container.querySelector('.session-tag') as HTMLElement
    tag.focus()
    fireEvent.keyDown(tag, { key: 'ContextMenu' })

    const menu = screen.getByRole('menu', { name: 'Session actions for forge-existing' })
    expect(menu).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Send to session' })).toHaveFocus()
    fireEvent.keyDown(menu, { key: 'ArrowDown' })
    expect(screen.getByRole('menuitem', { name: 'What this agent sees' })).toHaveFocus()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    expect(tag).toHaveFocus()

    // The nested remove control is not the tag, so the same key opens nothing.
    const remove = container.querySelector('.tag-remove') as HTMLButtonElement
    const shiftF10 = new KeyboardEvent('keydown', { key: 'F10', shiftKey: true, bubbles: true, cancelable: true })
    act(() => remove.dispatchEvent(shiftF10))

    expect(shiftF10.defaultPrevented).toBe(false)
    expect(document.querySelector('.menu-sheet')).toBeNull()

    // A kept-alive workspace going inactive takes its open menu with it.
    dispatchContextMenu(tagLabel('forge-existing'))
    expect(document.querySelector('.menu-sheet')).toBeInTheDocument()
    rerender(<TerminalWindow {...props} {...({ workspaceActive: false } as any)} />)
    expect(document.querySelector('.menu-sheet')).not.toBeInTheDocument()
  })

  it('uses the whole mounted session tag as the drag surface and keeps a stationary invisible placeholder', () => {
    draggableState.transform = { x: 32, y: 18 }
    draggableState.isDragging = true

    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    const tag = container.querySelector('.session-tag') as HTMLElement
    expect(tag).toHaveAttribute('title', 'Drag forge-existing (Unix user build)')
    expect(container.querySelector('.session-tag-drag-handle')).toBeNull()
    expect(tag.style.transform).toBe('')
    expect(tag.style.transition).toBe('none')
    expect(tag.style.opacity).toBe('0')

    fireEvent.pointerDown(tagLabel('forge-existing'), { pointerType: 'touch' })
    expect(draggableState.listeners.onPointerDown).toHaveBeenCalled()
  })

  it('keeps the nested remove control out of drag activation while ordinary tag clicks still activate', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{
          id: 'terminal3-window-0',
          boundSessions: ['forge-existing', 'shell-existing'],
          activeSession: 'forge-existing',
          colorIndex: 0,
        }}
      />
    )

    const remove = screen.getAllByRole('button', { name: '×' })[0]
    fireEvent.pointerDown(remove, { pointerType: 'mouse' })
    expect(draggableState.listeners.onPointerDown).not.toHaveBeenCalled()
    expect(setActiveSession).not.toHaveBeenCalled()
    expect(container.querySelector('.session-tag-drag-handle')).toBeNull()

    fireEvent.click(tagLabel('shell-existing'))
    expect(setActiveSession).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'shell-existing')
  })

  // The whole tag is the drag surface, so a press that drifts past the sensor's
  // threshold becomes a drag and dnd-kit swallows the click that would have
  // followed. Selecting on the press is what keeps an unsteady click working,
  // and the press still reaches the drag sensor.
  it('marks and shows the session whose tag is pressed, and only on the primary button', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{
          id: 'terminal3-window-0',
          boundSessions: ['forge-existing', 'shell-existing'],
          activeSession: 'forge-existing',
          colorIndex: 0,
        }}
      />
    )

    expect(tagLabel('forge-existing').closest('.session-tag')).toHaveClass('active')
    expect(tagLabel('shell-existing').closest('.session-tag')).not.toHaveClass('active')

    fireEvent.pointerDown(tagLabel('shell-existing'), { pointerType: 'mouse', button: 0 })

    expect(setActiveSession).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'shell-existing')
    expect(draggableState.listeners.onPointerDown).toHaveBeenCalled()

    setActiveSession.mockClear()
    fireEvent.pointerDown(tagLabel('forge-existing'), { pointerType: 'mouse', button: 0 })
    fireEvent.click(tagLabel('forge-existing'))

    expect(setActiveSession).not.toHaveBeenCalled()

    // The secondary button belongs to the menu, so it never moves the frame.
    fireEvent.pointerDown(tagLabel('shell-existing'), { pointerType: 'mouse', button: 2 })

    expect(setActiveSession).not.toHaveBeenCalled()
  })

  it('calls removeSessionFromWindow when the tag remove button is clicked', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: '×' }))

    expect(removeSessionFromWindow).toHaveBeenCalledWith(
      'terminal3',
      'terminal3-window-0',
      'build:forge-existing',
    )
  })

  it('offers drop feedback only where the drop would actually land', () => {
    droppableState.active = { data: { current: { type: 'session', sessionName: 'alpha', sessionKey: 'alice:alpha' } } }

    const idle = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    // A drag in flight somewhere else on screen leaves this window alone.
    expect(idle.container.querySelector('.terminal-drop-overlay')).toBeNull()
    expect(idle.container.querySelector('.terminal-window')).not.toHaveClass('drop-target')
    expect(screen.queryByText('Release to add')).not.toBeInTheDocument()
    idle.unmount()

    droppableState.isOver = true

    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    const body = container.querySelector('.terminal-window-body') as HTMLElement
    const overlay = container.querySelector('.terminal-drop-overlay') as HTMLElement
    expect(body).toContainElement(overlay)
    expect(overlay).toHaveStyle({ inset: '0', pointerEvents: 'none' })
    expect(overlay).toHaveTextContent('Release to add')
    expect(container.querySelector('.terminal-window')).toHaveClass('drop-target')

    // A tag hovering the window it already lives in has nowhere to land.
    droppableState.active = {
      data: {
        current: {
          type: 'tag',
          sessionName: 'forge-existing',
          sessionKey: 'build:forge-existing',
          sourceWorkspaceId: 'terminal3',
          sourceWindowId: 'terminal3-window-0',
        },
      },
    }
    const home = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />
    )

    expect(home.container.querySelector('.terminal-drop-overlay')).toBeNull()
    expect(home.container.querySelector('.terminal-window')).not.toHaveClass('drop-target')
  })

  it('exposes a one-click Send action in the header for the mounted active session', () => {
    const { rerender } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Send to session forge-existing' }))

    expect(openSendToSession).toHaveBeenCalledWith({ targetSessionKey: 'build:forge-existing' })

    // With nothing mounted there is no one to send to.
    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )
    expect(screen.queryByRole('button', { name: /Send to session/i })).not.toBeInTheDocument()
  })

  // What runs in a session is a mark in its own tag, not a line of prose in the
  // window controls: an agent gets its product mark, a shell gets nothing, and
  // anything else is named in its own words.
  it('marks the harness in each tag and says nothing at all for a shell', () => {
    const { container, rerender } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />
    )

    expect(container.querySelector('.session-tag [data-harness="codex"]')).not.toBeNull()
    expect(screen.getByTitle('tmux reports codex')).toBeInTheDocument()
    expect(container.textContent).not.toContain('foreground')

    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['alice:shell-existing'], activeSession: 'alice:shell-existing', colorIndex: 0 }}
      />
    )
    expect(container.querySelector('.session-tag [data-harness]')).toBeNull()
    expect(container.querySelector('.harness-command')).toBeNull()
    expect(container.textContent).not.toContain('bash')

    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['alice:napping-existing'], activeSession: 'alice:napping-existing', colorIndex: 0 }}
      />
    )
    expect(container.querySelector('.harness-command')).toHaveTextContent('sleep')

    // A binding tmux cannot resolve, or one two Unix users could answer to,
    // gets no mark rather than a guessed one.
    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['missing'], activeSession: 'missing', colorIndex: 0 }}
      />
    )
    expect(container.querySelector('.harness-mark, .harness-command')).toBeNull()

    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['shared-existing'], activeSession: 'shared-existing', colorIndex: 0 }}
      />
    )
    expect(container.querySelector('.harness-mark, .harness-command')).toBeNull()
  })

  // Prefixes are what these names share; the tail is what tells them apart, so
  // the tail is what survives a narrow tile.
  it('keeps the tail of a hyphenated tag name and clips only its head', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />
    )

    const label = container.querySelector('.session-tag .session-label') as HTMLElement
    expect(label).toHaveAttribute('title', 'forge-existing')
    expect(label.querySelector('.session-label-head')).toHaveTextContent('forge-')
    expect(label.querySelector('.session-label-tail')).toHaveTextContent('existing')
  })

  it('renders a window restored from a layout stored with the retired pending placeholder', () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal3: {
              windows: [
                { id: 'terminal3-window-0', boundSessions: ['INIT-PENDING'], activeSession: 'INIT-PENDING', colorIndex: 0 },
              ],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: false,
      settings: DEFAULT_SETTINGS,
    }))

    const restored = loadStoredState('desktop')!.workspaces.terminal3.windows[0]
    expect(restored).toEqual({ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 })

    render(<TerminalWindow workspaceId="terminal3" window={restored} />)

    expect(screen.getByText('or drag a session here')).toBeInTheDocument()
    expect(screen.queryByText(/INIT-PENDING/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Initializing Session/)).not.toBeInTheDocument()
  })

  it('holds an ended binding in its own tile, showing the last frame with Restart and Remove', () => {
    poolState.connectionStates = new Map([['alice:departed', 'closed']])
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{
          id: 'terminal3-window-0',
          boundSessions: ['alice:departed', 'shell-existing'],
          activeSession: 'alice:departed',
          colorIndex: 0,
        }}
      />
    )

    // The binding the operator was reading is still the one on screen; the live
    // session bound beside it is not promoted into the frame.
    expect(container.querySelector('.terminal-window-body')).toHaveAttribute('data-tile-state', 'ended')
    expect(setActiveSession).not.toHaveBeenCalled()
    expect(removeSessionFromWindow).not.toHaveBeenCalled()
    expect(screen.getByText(/departed ended/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Restart' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Remove' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Claim' })).not.toBeInTheDocument()
    expect(container.querySelector('.session-tag[data-tile-state="ended"] .tag-state')).toHaveTextContent('ended')

    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    expect(removeSessionFromWindow).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'alice:departed')
  })

  it('recreates an ended session in the same tile and dials the pooled terminal again', async () => {
    restartSession.mockResolvedValue(true)
    poolState.connectionStates = new Map([['alice:departed', 'closed']])
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['alice:departed'], activeSession: 'alice:departed', colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Restart' }))
    await waitFor(() => expect(restartSession).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'alice:departed'))
    await waitFor(() => expect(reconnect).toHaveBeenCalledWith('alice:departed'))
  })

  // A chrote-srv restart kills every pty, so every open tile's connection is
  // lost while every session stays alive (ADR-0013). Read as a takeover, that
  // told the operator twenty sessions were attached elsewhere when the socket
  // had one client, and made him click Reclaim once per tile.
  it('dials again for a tile whose connection was lost, rather than claiming it was taken over', () => {
    poolState.connectionStates = new Map([['shell-existing', 'dropped']])
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['shell-existing'], activeSession: 'shell-existing', colorIndex: 0 }}
      />
    )

    expect(redialIfDropped).toHaveBeenCalledWith('shell-existing')
    expect(screen.queryByText(/attached elsewhere/)).not.toBeInTheDocument()
    expect(container.querySelector('.terminal-window-body')).toHaveAttribute('data-tile-state', 'lost')
    // Until the dial lands the tile says what it knows, and offers the way back
    // itself instead of retrying.
    expect(screen.getByText(/shell-existing lost its connection/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Reconnect' }))
    expect(reconnect).toHaveBeenCalledWith('shell-existing')

    // A tile nobody is looking at costs a dial for nothing, so it waits.
    redialIfDropped.mockClear()
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['shell-existing'], activeSession: 'shell-existing', colorIndex: 0 }}
        workspaceActive={false}
      />
    )

    expect(redialIfDropped).not.toHaveBeenCalled()
  })

  it('offers Claim, not Restart, when the session is alive but another client detached this terminal', () => {
    poolState.connectionStates = new Map([['shell-existing', 'closed']])
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['shell-existing'], activeSession: 'shell-existing', colorIndex: 0 }}
      />
    )

    expect(container.querySelector('.terminal-window-body')).toHaveAttribute('data-tile-state', 'takenOver')
    expect(screen.getByText(/shell-existing was detached by another client/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Restart' })).not.toBeInTheDocument()

    // Claim dials again and takes the sizing seat, so the session comes back at
    // this device's size without detaching anyone watching it.
    fireEvent.click(screen.getByRole('button', { name: 'Claim' }))
    expect(claim).toHaveBeenCalledWith('shell-existing')
    expect(reconnect).not.toHaveBeenCalled()
  })

  it('leaves a bound session that is not on screen idle, with no detached affordance', () => {
    poolState.connectionStates = new Map([['alice:departed', 'closed']])
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{
          id: 'terminal3-window-0',
          boundSessions: ['alice:departed', 'shell-existing'],
          activeSession: 'shell-existing',
          colorIndex: 0,
        }}
        workspaceActive={false}
      />
    )

    expect(container.querySelector('.terminal-window-body')).toHaveAttribute('data-tile-state', 'idle')
    expect(container.querySelector('.terminal-tile-detached')).not.toBeInTheDocument()
    // The ended fact still shows on the tag it belongs to.
    expect(container.querySelector('.session-tag[data-tile-state="ended"] .tag-state')).toHaveTextContent('ended')
  })

  it('focuses only from the terminal body, never from its header', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(container.querySelector('.terminal-window-header')!)

    expect(setFocusedWindowKey).not.toHaveBeenCalled()

    expect(screen.getByText('Connecting…')).toBeInTheDocument()

    fireEvent.click(container.querySelector('.terminal-window-body')!)
    expect(setFocusedWindowKey).toHaveBeenCalledWith('terminal3-terminal3-window-0')
  })
})
