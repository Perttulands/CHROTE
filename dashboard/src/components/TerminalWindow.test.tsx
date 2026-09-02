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
]))
const pooledTerminals = new Map<string, { reconnect: () => void; fit: () => void; focus: () => void }>()

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
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({ ok: true })) as any)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
  })

  it('creates new sessions with the shared creation action and attaches to the current window', async () => {
    let finishCreate!: (sessionName: string) => void
    createSession.mockReturnValue(new Promise(resolve => { finishCreate = resolve }))
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    const createButton = screen.getByRole('button', { name: /New Session/i })
    // The shape a detached tile borrows for its own actions.
    expect(createButton).toHaveClass('tile-action-btn')
    fireEvent.click(createButton)

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createButton).toBeDisabled()
    expect(createButton).toHaveTextContent('...')
    fireEvent.click(createButton)
    expect(createSession).toHaveBeenCalledTimes(1)
    expect(createSession).toHaveBeenCalledWith({
      workspaceId: 'terminal3',
      attachTo: { workspaceId: 'terminal3', windowId: 'terminal3-window-0' },
    })
    await act(async () => finishCreate('forge1'))
    await waitFor(() => expect(createButton).toBeEnabled())
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
    expect(screen.getByText('shell-existing').closest('.session-tag')).toHaveClass('active')
    expect(screen.getByText('forge-existing').closest('.session-tag')).not.toHaveClass('active')
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

  it('does not intercept right-click on the empty-window new-session button', async () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    const event = dispatchContextMenu(screen.getByRole('button', { name: /New Session/i }))

    expect(event.defaultPrevented).toBe(false)
    expect(document.querySelector('.session-context-menu')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /New Session/i }))
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

    const openInactiveMenu = () => dispatchContextMenu(screen.getByText('shell-existing'))

    const sendEvent = openInactiveMenu()
    expect(sendEvent.defaultPrevented).toBe(true)
    const menuButtons = screen.getAllByRole('menuitem')
    expect(menuButtons).toHaveLength(6)
    for (const label of [
      'Send to session',
      'Reconnect frame',
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

    dispatchContextMenu(screen.getByText('pinned-existing'))
    const pinnedRefit = screen.getByRole('menuitem', { name: /^Refit frame/ })
    expect(pinnedRefit).toBeDisabled()
    expect(pinnedRefit).toHaveTextContent('Pinned at 100x30. tmux window-size is manual on this window, so CHROTE cannot resize it.')
    expect(pinnedRefit).toHaveAttribute('title', 'Pinned at 100x30. tmux window-size is manual on this window, so CHROTE cannot resize it.')
    fireEvent.click(pinnedRefit)
    expect(fit).not.toHaveBeenCalled()
    fireEvent.keyDown(document, { key: 'Escape' })

    dispatchContextMenu(screen.getByText('shell-existing'))
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

    const openMenu = () => dispatchContextMenu(screen.getByText('shell-existing'))

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
    expect(screen.getByText('shell-existing')).toBeInTheDocument()
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

    dispatchContextMenu(screen.getByText('shell-existing'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Reconnect frame' }))
    expect(reconnect).toHaveBeenCalledWith('shell-existing')

    dispatchContextMenu(screen.getByText('shell-existing'))
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
    dispatchContextMenu(screen.getByText('forge-existing'))
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

    dispatchContextMenu(screen.getByText('forge-existing'))
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

    dispatchContextMenu(screen.getByText('missing-session'))

    expect(screen.getByRole('menuitem', { name: 'Open files in working directory' })).toBeDisabled()
  })

  it('does not hide a Send action behind ctrl-click on an attached session tag', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByText('forge-existing'), { ctrlKey: true })

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

    fireEvent.pointerDown(screen.getByText('forge-existing'), { pointerType: 'touch' })
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

    fireEvent.click(screen.getByText('forge-existing'))
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

    const tagNameRule = rule('.tag-name')
    expect(tagNameRule).toMatch(/\boverflow:\s*hidden;/)
    expect(tagNameRule).toMatch(/\btext-overflow:\s*ellipsis;/)
    expect(tagNameRule).toMatch(/\bwhite-space:\s*nowrap;/)
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

  it('shows the mounted session foreground command without treating attachment as agent liveness', () => {
    const { rerender } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['build:forge-existing'], activeSession: 'build:forge-existing', colorIndex: 0 }}
      />
    )

    expect(screen.getByTitle('Foreground process reported by tmux: codex')).toHaveTextContent('foreground: codex')

    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['alice:shell-existing'], activeSession: 'alice:shell-existing', colorIndex: 0 }}
      />
    )
    expect(screen.getByTitle('Foreground process reported by tmux: bash')).toHaveTextContent('foreground: shell')
  })

  it('does not invent foreground evidence for unknown or user-ambiguous session bindings', () => {
    const { rerender } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['missing'], activeSession: 'missing', colorIndex: 0 }}
      />
    )
    expect(screen.queryByTitle(/Foreground process reported by tmux:/)).not.toBeInTheDocument()

    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['shared-existing'], activeSession: 'shared-existing', colorIndex: 0 }}
      />
    )
    expect(screen.queryByTitle(/Foreground process reported by tmux:/)).not.toBeInTheDocument()
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

  it('keeps terminal panes on the per-window opaque background palette', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 2 }}
      />
    )

    const terminalWindow = container.querySelector('.terminal-window') as HTMLElement
    expect(terminalWindow.style.getPropertyValue('--window-bg')).toBe('rgba(10, 26, 10, 0.85)')

    const css = terminalCss()
    expect(css).toContain('background-color: var(--window-bg, var(--surface-primary));')
    expect(css).toContain('background-color: var(--window-bg, rgba(0, 0, 0, 0.5));')
    expect(css).not.toContain('background-image: url(')
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
    expect(screen.queryByRole('button', { name: 'Reclaim' })).not.toBeInTheDocument()
    expect(container.querySelector('.session-tag[data-tile-state="ended"] .tag-state')).toHaveTextContent('ended')

    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    expect(removeSessionFromWindow).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'alice:departed')
  })

  it('states a detached tile in the middle of the frame, in the button shape an empty window already uses', () => {
    poolState.connectionStates = new Map([['alice:departed', 'closed']])
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['alice:departed'], activeSession: 'alice:departed', colorIndex: 0 }}
      />
    )

    // Both actions carry the shape the New Session button carries; the bespoke
    // third shape the tile used to draw is gone from markup and stylesheet.
    const actions = [...container.querySelectorAll('.terminal-tile-detached-actions button')]
    expect(actions.map(button => button.textContent)).toEqual(['Restart', 'Remove'])
    actions.forEach(button => expect(button).toHaveClass('tile-action-btn'))
    expect(container.querySelector('.terminal-tile-action')).toBeNull()

    const css = terminalCss()
    expect(css).not.toContain('.terminal-tile-action')
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

    // The New Session button is the same shape, one size up.
    expect(rule('.create-session-btn')).toMatch(/\bpadding:\s*24px 32px;/)
    expect(rule('.tile-action-btn-compact')).toMatch(/\bpadding:\s*7px 14px;/)
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

  it('offers Reclaim, not Restart, when the session is alive but the connection was taken', () => {
    poolState.connectionStates = new Map([['shell-existing', 'closed']])
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['shell-existing'], activeSession: 'shell-existing', colorIndex: 0 }}
      />
    )

    expect(container.querySelector('.terminal-window-body')).toHaveAttribute('data-tile-state', 'takenOver')
    expect(screen.getByText(/shell-existing is attached elsewhere/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Restart' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Reclaim' }))
    expect(reconnect).toHaveBeenCalledWith('shell-existing')
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
