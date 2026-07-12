import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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
const setFocusedWindowKey = vi.fn()
const setActiveSession = vi.fn()
const openSendToSession = vi.fn()
const draggableState = vi.hoisted(() => ({
  transform: null as { x: number; y: number } | null,
  isDragging: false,
  listeners: { onPointerDown: vi.fn() },
}))

vi.mock('@dnd-kit/core', () => ({
  useDraggable: () => ({
    attributes: { role: 'button', tabIndex: 0 },
    listeners: draggableState.listeners,
    setNodeRef: vi.fn(),
    transform: draggableState.transform,
    isDragging: draggableState.isDragging,
  }),
  useDroppable: () => ({ setNodeRef: vi.fn(), isOver: false }),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: {
      ...DEFAULT_SETTINGS,
      terminalLaunchUsers: {
        ...DEFAULT_SETTINGS.terminalLaunchUsers,
        terminal3: 'tavern',
      },
      terminalSessionPrefixes: {
        ...DEFAULT_SETTINGS.terminalSessionPrefixes,
        tavern: 'forge',
      },
    },
    terminalUsers: ['perttu', 'tavern'],
    sessions: [
      { name: 'forge-existing', windows: 1, attached: false, group: 'forge', unixUser: 'tavern' },
      { name: 'shell-existing', windows: 1, attached: false, group: 'shell', unixUser: 'perttu' },
    ],
    layoutPresets: [{ id: 'preset-1', name: 'Focus Layout', createdAt: 1, workspaces: {} }],
    refreshSessions,
    createSession,
    addSessionToWindow,
    removeSessionFromWindow,
    setActiveSession,
    cycleSession: vi.fn(),
    setWindowCount: vi.fn(),
    clearStaleSessionsFromWindow: vi.fn(),
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
    claimIframe: vi.fn(() => vi.fn()),
    isLoaded: vi.fn(() => false),
    getIframe: vi.fn(() => null),
    triggerFit: vi.fn(),
    focusIframe: vi.fn(),
    reconnectIframe,
  }),
}))

const testDir = dirname(fileURLToPath(import.meta.url))
const terminalCss = () => readFileSync(resolve(testDir, '../styles/terminal.css'), 'utf8')

function dispatchContextMenu(target: Element) {
  const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 350, clientY: 230 })
  target.dispatchEvent(event)
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
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
      />
    )

    const event = dispatchContextMenu(screen.getByRole('button', { name: /New Session/i }))

    expect(event.defaultPrevented).toBe(false)
    expect(container.querySelector('.session-context-menu')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /New Session/i }))
    await waitFor(() => expect(createSession).toHaveBeenCalled())
  })

  it('does not intercept right-click on session tags', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    const event = dispatchContextMenu(screen.getByText('forge-existing'))

    expect(event.defaultPrevented).toBe(false)
    expect(container.querySelector('.session-context-menu')).toBeNull()
    expect(screen.queryByRole('button', { name: /Rename/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Kill/i })).not.toBeInTheDocument()
  })

  it('opens Send to Session from ctrl-click on an attached session tag', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['tavern:forge-existing'], activeSession: 'tavern:forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByText('forge-existing'), { ctrlKey: true })

    expect(openSendToSession).toHaveBeenCalledWith('tavern:forge-existing')
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
    expect(container.querySelector('.session-context-menu')).toBeNull()
  })

  it('uses a pointer-only non-interactive tag grip and leaves the dragged tag as a stationary invisible placeholder', () => {
    draggableState.transform = { x: 32, y: 18 }
    draggableState.isDragging = true

    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    const tag = container.querySelector('.session-tag') as HTMLElement
    const handle = container.querySelector('.session-tag-drag-handle') as HTMLElement
    expect(handle.tagName).toBe('SPAN')
    expect(handle).toHaveAttribute('aria-hidden', 'true')
    expect(handle).toHaveAttribute('title', 'Drag forge-existing (Unix user tavern)')
    expect(handle).not.toHaveAttribute('role')
    expect(handle).not.toHaveAttribute('tabindex')
    expect(handle).not.toHaveAttribute('aria-roledescription')
    expect(handle).not.toHaveAttribute('aria-describedby')
    expect(tag.style.transform).toBe('')
    expect(tag.style.transition).toBe('none')
    expect(tag.style.opacity).toBe('0')

    fireEvent.pointerDown(handle, { pointerType: 'touch' })
    expect(draggableState.listeners.onPointerDown).toHaveBeenCalled()
  })

  it('keeps tag grip clicks inert while ordinary tag clicks still activate the session', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    const handle = container.querySelector('.session-tag-drag-handle') as HTMLElement
    fireEvent.click(handle)
    expect(setActiveSession).not.toHaveBeenCalled()

    fireEvent.click(screen.getByText('forge-existing'))
    expect(setActiveSession).toHaveBeenCalledWith('terminal3', 'terminal3-window-0', 'forge-existing')
  })

  it('calls removeSessionFromWindow when the tag remove button is clicked', () => {
    render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['tavern:forge-existing'], activeSession: 'tavern:forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: '×' }))

    expect(removeSessionFromWindow).toHaveBeenCalledWith(
      'terminal3',
      'terminal3-window-0',
      'tavern:forge-existing',
    )
  })

  it('renders full-body visual drop feedback without making the overlay the hit target', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }}
        isDragging
      />
    )

    const body = container.querySelector('.terminal-window-body') as HTMLElement
    const overlay = container.querySelector('.terminal-drop-overlay') as HTMLElement
    expect(body).toContainElement(overlay)
    expect(overlay).toHaveStyle({ inset: '0', pointerEvents: 'none' })
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

  it('does not focus from generic header clicks but focuses when clicking the status dot or body', () => {
    const { container } = render(
      <TerminalWindow
        workspaceId="terminal3"
        window={{ id: 'terminal3-window-0', boundSessions: ['forge-existing'], activeSession: 'forge-existing', colorIndex: 0 }}
      />
    )

    fireEvent.click(container.querySelector('.terminal-window-header')!)

    expect(setFocusedWindowKey).not.toHaveBeenCalled()

    fireEvent.click(container.querySelector('.status-dot')!)
    expect(setFocusedWindowKey).toHaveBeenCalledWith('terminal3-terminal3-window-0')

    setFocusedWindowKey.mockClear()

    fireEvent.click(container.querySelector('.terminal-window-body')!)
    expect(setFocusedWindowKey).toHaveBeenCalledWith('terminal3-terminal3-window-0')
  })
})
