import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import TerminalWindow from './TerminalWindow'
import { DEFAULT_SETTINGS } from '../types'
import { sessionEvidenceFrom } from '../terminal/tileState'
import { loadStoredState } from '../context/workspaceLayouts'

const refreshSessions = vi.fn()
const createSession = vi.fn()
const addSessionToWindow = vi.fn()
const addToast = vi.fn()
const removeSessionFromWindow = vi.fn()
const renameSession = vi.fn()
const deleteSession = vi.fn()
const reconnect = vi.fn()
const claim = vi.fn()
const redialIfDropped = vi.fn()
const fit = vi.fn()
const setFocusedWindowKey = vi.fn()
const setActiveSession = vi.fn()
const cycleSession = vi.fn()
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
    cycleSession,
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

vi.mock('../context/ToastContext', () => ({
  useToast: () => ({ addToast }),
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

const testDir = dirname(fileURLToPath(import.meta.url))
const terminalCss = () => readFileSync(resolve(testDir, './TerminalWorkspaceDock.css'), 'utf8')

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

  it('shows cycle controls only for multiple sessions and marks the active tag', () => {
    const twoSessions = {
      id: 'terminal3-window-0',
      boundSessions: ['build:forge-existing', 'alice:shell-existing'],
      activeSession: 'alice:shell-existing',
      colorIndex: 0,
    }
    const { container, rerender } = render(<TerminalWindow workspaceId="terminal3" window={twoSessions} />)

    expect(screen.getByTitle('Previous session')).toBeInTheDocument()
    expect(screen.getByTitle('Next session')).toBeInTheDocument()
    expect(tagLabel('shell-existing').closest('.session-tag')).toHaveClass('active')
    expect(tagLabel('forge-existing').closest('.session-tag')).not.toHaveClass('active')
    fireEvent.click(screen.getByTitle('Next session'))
    expect(cycleSession).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'next')

    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ ...twoSessions, boundSessions: ['alice:shell-existing'] }}
      />
    )
    expect(container.querySelectorAll('.cycle-btn')).toHaveLength(0)
  })

  it('does not intercept right-click on the empty window launcher', async () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    const launchButton = await screen.findByRole('button', { name: 'Launch claude in chrote' })
    const event = dispatchContextMenu(launchButton)

    expect(event.defaultPrevented).toBe(false)
    expect(document.querySelector('.session-context-menu')).toBeNull()
    fireEvent.click(launchButton)
    await waitFor(() => expect(createSession).toHaveBeenCalled())
  })

  it('opens only the requested actions for the clicked session tag', () => {
    const openFilesAtPath = vi.fn()
    render(
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
    expect(menuButtons).toHaveLength(7)
    for (const label of [
      'Send to session',
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
    expect(openSendToSession).toHaveBeenCalledWith('alice:shell-existing')

    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Reconnect frame' }))
    expect(reconnect).toHaveBeenCalledWith('alice:shell-existing')

    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Refit frame' }))
    expect(fit).toHaveBeenCalledWith('alice:shell-existing')

    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Open files in working directory' }))
    expect(openFilesAtPath).toHaveBeenCalledWith('/srv/live-shell')

    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Kill session' }))
    expect(deleteSession).toHaveBeenCalledWith('shell-existing', 'alice')

    expect(setActiveSession).not.toHaveBeenCalled()
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

  it('opens the tag menu from the keyboard and restores focus on Escape', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />,
    )
    const tag = container.querySelector('.session-tag') as HTMLElement
    tag.focus()
    fireEvent.keyDown(tag, { key: 'ContextMenu' })

    const menu = screen.getByRole('menu', { name: 'Session actions for forge-existing' })
    expect(menu).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Send to session' })).toHaveFocus()
    fireEvent.keyDown(menu, { key: 'ArrowDown' })
    expect(screen.getByRole('menuitem', { name: 'Reconnect frame' })).toHaveFocus()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    expect(tag).toHaveFocus()
  })

  it('does not open tag actions from Shift+F10 on the nested remove control', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />,
    )
    const remove = container.querySelector('.tag-remove') as HTMLButtonElement
    const event = new KeyboardEvent('keydown', {
      key: 'F10',
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    })

    act(() => remove.dispatchEvent(event))

    expect(event.defaultPrevented).toBe(false)
    expect(document.querySelector('.session-context-menu')).toBeNull()
  })

  it('clears an open tag menu when its keep-alive workspace becomes inactive', () => {
    const props = {
      workspaceId: 'terminal3' as const,
      window: { id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 },
    }
    const { rerender } = render(<TerminalWindow {...props} {...({ workspaceActive: true } as any)} />)
    dispatchContextMenu(tagLabel('forge-existing'))
    expect(document.querySelector('.session-context-menu')).toBeInTheDocument()

    rerender(<TerminalWindow {...props} {...({ workspaceActive: false } as any)} />)
    expect(document.querySelector('.session-context-menu')).not.toBeInTheDocument()
  })

  it('opens the live session working directory', () => {
    const openFilesAtPath = vi.fn()
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
        onOpenFilesAtPath={openFilesAtPath}
      />
    )

    dispatchContextMenu(tagLabel('forge-existing'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Open files in working directory' }))

    expect(openFilesAtPath).toHaveBeenCalledWith('/srv/forge')
  })

  it('disables working-directory routing when the clicked session has no reported cwd', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['missing-session'], activeSession: 'missing-session', colorIndex: 0 }}
        onOpenFilesAtPath={vi.fn()}
      />
    )

    dispatchContextMenu(tagLabel('missing-session'))

    expect(screen.getByRole('menuitem', { name: 'Open files in working directory' })).toBeDisabled()
  })

  it('does not hide a Send action behind ctrl-click on an attached session tag', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(tagLabel('forge-existing'), { ctrlKey: true })

    expect(openSendToSession).not.toHaveBeenCalled()
    expect(setActiveSession).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'build:forge-existing')
  })

  it('does not intercept right-click on terminal window chrome', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    const event = dispatchContextMenu(container.querySelector('.terminal-window-header') as HTMLElement)

    expect(event.defaultPrevented).toBe(false)
    expect(document.querySelector('.session-context-menu')).toBeNull()
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
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    const remove = screen.getByRole('button', { name: '×' })
    fireEvent.pointerDown(remove, { pointerType: 'mouse' })
    expect(draggableState.listeners.onPointerDown).not.toHaveBeenCalled()
    expect(container.querySelector('.session-tag-drag-handle')).toBeNull()

    fireEvent.click(tagLabel('forge-existing'))
    expect(setActiveSession).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'forge-existing')
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

  it('shares terminal header width equally between every tag without crowding controls', () => {
    const css = terminalCss()
    const rule = (selector: string) => {
      const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      const match = css.match(new RegExp(`${escapedSelector}\\s*\\{([^}]*)\\}`))
      expect(match, `missing CSS rule ${selector}`).not.toBeNull()
      return match![1]
    }

    expect(rule('.session-tags')).toMatch(/\bflex:\s*1;/)
    expect(rule('.session-tags')).toMatch(/\bgap:\s*4px;/)

    const tagRule = rule('.session-tag')
    expect(tagRule).toMatch(/\bflex:\s*1 1 0;/)
    expect(tagRule).toMatch(/\bmin-width:\s*0;/)
    expect(tagRule).not.toMatch(/\bmax-width:/)
    expect(rule('.session-tag.active')).not.toMatch(/\b(?:flex(?:-(?:basis|grow|shrink))?|min-width|max-width|width)\s*:/)

    // The name truncates from the head, so the tail of a prefixed name always
    // reads. Those rules are shared with the session list, in base.css.
    const shared = readFileSync(resolve(testDir, '../styles/base.css'), 'utf8')
    const sharedRule = (selector: string) => {
      const match = shared.match(new RegExp(`${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`))
      expect(match, `missing CSS rule ${selector}`).not.toBeNull()
      return match![1]
    }
    const headRule = sharedRule('.session-label-head')
    expect(headRule).toMatch(/\boverflow:\s*hidden;/)
    expect(headRule).toMatch(/\btext-overflow:\s*ellipsis;/)
    expect(sharedRule('.session-label')).toMatch(/\bwhite-space:\s*nowrap;/)
    expect(sharedRule('.session-label-tail')).toMatch(/\bflex:\s*0 0 auto;/)
    expect(rule('.tag-remove')).toMatch(/\bflex-shrink:\s*0;/)
    expect(rule('.window-controls')).toMatch(/\bflex:\s*0 0 auto;/)
  })

  it('stays calm during a drag until this window is actually hovered', () => {
    droppableState.active = { data: { current: { type: 'session', sessionName: 'alpha', sessionKey: 'alice:alpha' } } }

    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    expect(container.querySelector('.terminal-drop-overlay')).toBeNull()
    expect(container.querySelector('.terminal-window')).not.toHaveClass('drop-target')
    expect(screen.queryByText('Release to add')).not.toBeInTheDocument()
  })

  it('renders hovered-target drop feedback without making the overlay the hit target', () => {
    droppableState.active = { data: { current: { type: 'session', sessionName: 'alpha', sessionKey: 'alice:alpha' } } }
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
  })

  it('shows no drop feedback when hovering a tag over its own source window', () => {
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
    droppableState.isOver = true

    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />
    )

    expect(container.querySelector('.terminal-drop-overlay')).toBeNull()
    expect(container.querySelector('.terminal-window')).not.toHaveClass('drop-target')
  })

  it('exposes a one-click Send action in the header for the mounted active session', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Send to session forge-existing' }))

    expect(openSendToSession).toHaveBeenCalledWith('build:forge-existing')
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
  })

  it('marks nothing for unknown or user-ambiguous session bindings', () => {
    const { container, rerender } = render(
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

  it('offers no Send action for an empty window', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )
    expect(screen.queryByRole('button', { name: /Send to session/i })).not.toBeInTheDocument()
  })

  // The four coloured window themes are gone. A tile is a surface and a
  // divider, focus is the accent border, and nothing about the tile says which
  // slot in the grid it happens to sit in.
  it('paints every tile on the same surface, whatever its stored window index', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 2 }}
      />
    )

    const terminalWindow = container.querySelector('.terminal-window') as HTMLElement
    expect(terminalWindow.style.getPropertyValue('--window-bg')).toBe('')
    expect(terminalWindow.style.getPropertyValue('--window-accent')).toBe('')
    expect(terminalWindow.style.getPropertyValue('--window-border')).toBe('')

    const css = terminalCss()
    expect(css).not.toContain('--window-')
    expect(css).toContain('background-color: var(--surface-primary);')
    expect(css).toContain('background-color: rgba(0, 0, 0, 0.5);')
    expect(css).not.toContain('background-image: url(')
    expect(css).toMatch(/\.terminal-window\.focused \{\s*border-color: var\(--accent\);\s*\}/)
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

  it('states a detached tile in the middle of the frame, with plain outline controls', () => {
    poolState.connectionStates = new Map([['alice:departed', 'closed']])
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['alice:departed'], activeSession: 'alice:departed', colorIndex: 0 }}
      />
    )

    // Reclaiming a tile is ordinary work, so both actions are ordinary outline
    // buttons rather than the dashed accent shape an empty window offers.
    const actions = [...container.querySelectorAll('.terminal-tile-detached-actions button')]
    expect(actions.map(button => button.textContent)).toEqual(['Restart', 'Remove'])
    actions.forEach(button => expect(button).toHaveClass('terminal-tile-detached-action'))
    actions.forEach(button => expect(button).not.toHaveClass('tile-action-btn'))
    expect(container.querySelector('.terminal-tile-action')).toBeNull()

    const css = terminalCss()
    expect(css).not.toContain('.terminal-tile-action')
    expect(css).not.toContain('tile-action-btn-compact')
    const rule = (selector: string) => {
      const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      const match = css.match(new RegExp(`${escapedSelector}\\s*\\{([^}]*)\\}`))
      expect(match, `missing CSS rule ${selector}`).not.toBeNull()
      return match![1]
    }

    // Centred in the frame, not pinned along its bottom edge, and sized by its
    // own content so the last rendered frame stays readable behind it.
    const panel = rule('.terminal-tile-detached')
    expect(panel).toMatch(/\btop:\s*50%;/)
    expect(panel).toMatch(/\btransform:\s*translate\(-50%, -50%\);/)
    expect(panel).not.toMatch(/\bbottom:\s*0;/)
    expect(panel).not.toMatch(/\bright:\s*0;/)
    expect(panel).toMatch(/\bmax-width:\s*calc\(100% - 24px\);/)

    // A plain outline: divider border, secondary text, no dashes, no shouting.
    const action = rule('.terminal-tile-detached-action')
    expect(action).toMatch(/\bborder:\s*1px solid var\(--divider\);/)
    expect(action).toMatch(/\bcolor:\s*var\(--text-secondary\);/)
    expect(action).toMatch(/\bborder-radius:\s*4px;/)
    expect(action).not.toMatch(/\btext-transform:/)
    expect(action).not.toMatch(/\bletter-spacing:/)
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
  })

  // This used to be held back: dialling attached with -d and would have thrown
  // the SSH client out. Nothing attaches with -d now, so the dial joins them
  // and costs them neither their client nor their size.
  it('dials again for a session another client is attached to, because that no longer evicts them', () => {
    poolState.connectionStates = new Map([['alice:ssh-held', 'dropped']])
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['alice:ssh-held'], activeSession: 'alice:ssh-held', colorIndex: 0 }}
      />
    )

    expect(container.querySelector('.terminal-window-body')).toHaveAttribute('data-tile-state', 'lost')
    expect(redialIfDropped).toHaveBeenCalledWith('alice:ssh-held')
  })

  it('does not dial again for a lost tile that is not on screen', () => {
    poolState.connectionStates = new Map([['shell-existing', 'dropped']])
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

  it('removes the misleading status dot and focuses only from the terminal body', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(container.querySelector('.terminal-window-header')!)

    expect(setFocusedWindowKey).not.toHaveBeenCalled()

    expect(container.querySelector('.status-dot')).not.toBeInTheDocument()
    expect(screen.getByText('Connecting…')).toBeInTheDocument()

    fireEvent.click(container.querySelector('.terminal-window-body')!)
    expect(setFocusedWindowKey).toHaveBeenCalledWith('terminal3-terminal3-window-0')
  })
})
