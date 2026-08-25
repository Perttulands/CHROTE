import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import TerminalWindow from './TerminalWindow'
import { DEFAULT_SETTINGS } from '../types'

const refreshSessions = vi.fn()
const createSession = vi.fn()
const addSessionToWindow = vi.fn()
const addToast = vi.fn()
const removeSessionFromWindow = vi.fn()
const renameSession = vi.fn()
const deleteSession = vi.fn()
const reconnectIframe = vi.fn()
const triggerFit = vi.fn()
const setFocusedWindowKey = vi.fn()
const setActiveSession = vi.fn()
const openSendToSession = vi.fn()
const clearStaleSessionsFromWindow = vi.fn()
const claimIframe = vi.fn(() => vi.fn())
const recoveryState = vi.hoisted(() => ({ evidence: [] as Array<Record<string, unknown>> }))
const draggableState = vi.hoisted(() => ({
  transform: null as { x: number, y: number } | null,
  isDragging: false,
  listeners: { onPointerDown: vi.fn() },
}))
const droppableState = vi.hoisted(() => ({
  isOver: false,
  active: null as { data: { current: Record<string, unknown> } } | null,
}))
const poolState = vi.hoisted(() => ({ loadedSessions: new Set<string>() }))

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
    sessions: [
      { name: 'forge-existing', windows: 1, attached: false, group: 'forge', unixUser: 'build' },
      { name: 'shell-existing', windows: 1, attached: false, group: 'shell', unixUser: 'alice', cwd: '/srv/live-shell' },
    ],
    sessionBank: [
      { name: 'forge-existing', unixUser: 'build', cwd: '/srv/forge' },
      { name: 'shell-existing', unixUser: 'alice', cwd: '/srv/shell' },
    ],
    recoveryEvidence: recoveryState.evidence,
    layoutPresets: [{ id: 'preset-1', name: 'Focus Layout', createdAt: 1, workspaces: {} }],
    refreshSessions,
    createSession,
    addSessionToWindow,
    removeSessionFromWindow,
    setActiveSession,
    cycleSession: vi.fn(),
    setWindowCount: vi.fn(),
    clearStaleSessionsFromWindow,
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

vi.mock('./IframePool', () => ({
  useIframePool: () => ({
    claimIframe,
    isLoaded: vi.fn(() => false),
    loadedSessions: poolState.loadedSessions,
    getIframe: vi.fn(() => null),
    triggerFit,
    focusIframe: vi.fn(),
    reconnectIframe,
  }),
}))

const testDir = dirname(fileURLToPath(import.meta.url))
const terminalCss = () => readFileSync(resolve(testDir, '../styles/terminal.css'), 'utf8')

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
    poolState.loadedSessions = new Set()
    recoveryState.evidence = []
    clearStaleSessionsFromWindow.mockClear()
    claimIframe.mockClear()
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({ ok: true })) as any)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
  })

  it('creates new sessions with the shared creation action and attaches to the current window', async () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: /New Session/i }))

    await waitFor(() => expect(createSession).toHaveBeenCalled())
    expect(createSession).toHaveBeenCalledWith({
      workspaceId: 'terminal3',
      attachTo: { workspaceId: 'terminal3', windowId: 'terminal3-window-0' },
    })
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
    expect(reconnectIframe).toHaveBeenCalledWith('alice:shell-existing')

    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Refit frame' }))
    expect(triggerFit).toHaveBeenCalledWith('alice:shell-existing')

    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Open files in working directory' }))
    expect(openFilesAtPath).toHaveBeenCalledWith('/srv/live-shell')

    openInactiveMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Kill session' }))
    expect(deleteSession).toHaveBeenCalledWith('shell-existing', 'alice')

    expect(setActiveSession).not.toHaveBeenCalled()
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

  it('uses the original bound key for iframe actions on supported bare legacy tags', () => {
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
    expect(reconnectIframe).toHaveBeenCalledWith('shell-existing')

    dispatchContextMenu(screen.getByText('shell-existing'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Refit frame' }))
    expect(triggerFit).toHaveBeenCalledWith('shell-existing')
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

  it('does not expose session actions for the INIT-PENDING placeholder', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['INIT-PENDING'], activeSession: 'INIT-PENDING', colorIndex: 0 }}
      />,
    )

    const event = dispatchContextMenu(screen.getByText('INIT-PENDING'))

    expect(event.defaultPrevented).toBe(false)
    expect(document.querySelector('.session-context-menu')).toBeNull()
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

  it('falls back to a banked cwd when the live session has no reported cwd', () => {
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

  it('offers no Send action for empty or still-initializing windows', () => {
    const { rerender } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )
    expect(screen.queryByRole('button', { name: /Send to session/i })).not.toBeInTheDocument()

    rerender(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['INIT-PENDING'], activeSession: 'INIT-PENDING', colorIndex: 0 }}
      />
    )
    expect(screen.queryByRole('button', { name: /Send to session/i })).not.toBeInTheDocument()
  })

  it('renders bounded offline evidence and clears placement only on explicit action', () => {
    recoveryState.evidence = [{
      sourceId: 'tmux:alice',
      unixUser: 'alice',
      name: 'offline-agent',
      state: 'offline',
      cwd: '/srv/offline-agent',
    }]

    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['alice:offline-agent'], activeSession: 'alice:offline-agent', colorIndex: 0 }}
      />
    )

    expect(screen.getByText('Session is offline')).toBeInTheDocument()
    expect(screen.getByText('/srv/offline-agent')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Send to session/i })).not.toBeInTheDocument()
    expect(claimIframe).not.toHaveBeenCalledWith('alice:offline-agent', expect.anything())

    fireEvent.click(screen.getByRole('button', { name: 'Clear offline placement' }))
    expect(removeSessionFromWindow).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'alice:offline-agent')
  })

  it('matches one unambiguous qualified offline identity to a legacy bare binding', () => {
    recoveryState.evidence = [{
      sourceId: 'tmux:alice',
      unixUser: 'alice',
      name: 'offline-agent',
      state: 'offline',
      cwd: '/srv/offline-agent',
    }]

    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['offline-agent'], activeSession: 'offline-agent', colorIndex: 0 }}
      />
    )

    expect(screen.getByText('Session is offline')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Clear offline placement' }))
    expect(removeSessionFromWindow).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'offline-agent')
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
    expect(screen.getByText('Loading terminal…')).toBeInTheDocument()

    fireEvent.click(container.querySelector('.terminal-window-body')!)
    expect(setFocusedWindowKey).toHaveBeenCalledWith('terminal3-terminal3-window-0')
  })
})
