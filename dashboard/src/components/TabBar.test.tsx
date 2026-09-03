import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TabBar from './TabBar'
import { DEFAULT_SETTINGS, TERMINAL_WORKSPACE_IDS } from '../types'

const mockState = vi.hoisted(() => ({
  updateSettings: vi.fn(),
  saveCurrentLayout: vi.fn(),
  loadPreset: vi.fn(),
  clearWorkspaceAssignments: vi.fn(),
  deletePreset: vi.fn(),
  setWindowCount: vi.fn(),
  workspaceIds: null as readonly string[] | null,
  windowCount: 2,
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: DEFAULT_SETTINGS,
    updateSettings: mockState.updateSettings,
    saveCurrentLayout: mockState.saveCurrentLayout,
    loadPreset: mockState.loadPreset,
    layoutPresets: [{ id: 'preset-1', name: 'Focus Layout' }],
    clearWorkspaceAssignments: mockState.clearWorkspaceAssignments,
    deletePreset: mockState.deletePreset,
    setWindowCount: mockState.setWindowCount,
    workspaceIds: mockState.workspaceIds ?? TERMINAL_WORKSPACE_IDS,
    workspaces: {
      terminal1: {
        windowCount: mockState.windowCount,
        windows: [
          { id: 'terminal1-window-0', boundSessions: ['alice:alpha', 'bare-session'], activeSession: 'alice:alpha', colorIndex: 0 },
          { id: 'terminal1-window-1', boundSessions: ['bob:beta'], activeSession: 'bob:beta', colorIndex: 1 },
          { id: 'terminal1-window-2', boundSessions: [], activeSession: null, colorIndex: 2 },
          { id: 'terminal1-window-3', boundSessions: ['alice:hidden'], activeSession: 'alice:hidden', colorIndex: 3 },
        ],
      },
    },
  }),
}))

const reconnect = vi.fn()
const claim = vi.fn()
const pooledTerminals = new Map<string, { reconnect: () => void; claim: () => void }>()
vi.mock('./TerminalPool', () => ({
  useTerminalPool: () => ({
    terminals: {
      get(sessionKey: string) {
        if (!pooledTerminals.has(sessionKey)) {
          pooledTerminals.set(sessionKey, {
            reconnect: () => reconnect(sessionKey),
            claim: () => claim(sessionKey),
          })
        }
        return pooledTerminals.get(sessionKey)
      },
    },
    connectionStates: new Map(),
  }),
}))

/** The active terminal tab is the trigger: its caret opens the tab's menu. */
function openTabMenu() {
  const caret = document.querySelector('.tab.active .tab-menu-caret')
  if (!caret) throw new Error('the active terminal tab has no menu caret')
  fireEvent.click(caret)
}

function mockMatchMedia(matches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  })
}

describe('TabBar navigation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockState.deletePreset = vi.fn()
    mockState.setWindowCount = vi.fn()
    mockState.windowCount = 2
    pooledTerminals.clear()
  })

  it('draws the resolved workspace list and every other tab, and routes each one', () => {
    mockMatchMedia(false)
    const onTabChange = vi.fn()

    // The active tab carries its menu caret; the label is the word before it.
    const terminalLabels = () => screen.getAllByRole('button')
      .map(button => button.textContent?.replace('▾', ''))
      .filter(label => label?.startsWith('Terminal'))

    const { unmount } = render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)

    expect(terminalLabels()).toEqual(['Terminal', 'Terminal 2', 'Terminal 3'])
    expect(screen.getByRole('button', { name: 'Settings' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Terminal 3' }))
    expect(onTabChange).toHaveBeenLastCalledWith('terminal3')
    unmount()

    // The strip follows whatever ids the layout resolved, not a fixed tab set.
    mockState.workspaceIds = ['terminal1', 'terminal2', 'terminal3', 'terminal4', 'terminal5']
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    expect(terminalLabels()).toEqual(['Terminal', 'Terminal 2', 'Terminal 3', 'Terminal 4', 'Terminal 5'])
    mockState.workspaceIds = null
  })

  it('shows the Library in desktop navigation', () => {
    mockMatchMedia(false)
    const onTabChange = vi.fn()

    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Library' }))

    expect(onTabChange).toHaveBeenCalledWith('library')
  })

  it('shows Scheduled Tasks in desktop navigation', () => {
    mockMatchMedia(false)
    const onTabChange = vi.fn()

    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Scheduled' }))

    expect(onTabChange).toHaveBeenCalledWith('scheduled')
  })

  // A phone has no room for the tab strip, so the hamburger is the only way to
  // every tab: none of them is reachable until it is opened.
  it('reaches every tab through the hamburger on a phone', () => {
    mockMatchMedia(true)
    const onTabChange = vi.fn()

    const { container } = render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    expect(container.querySelector('.tab-bar')).toHaveClass('mobile-mode')
    expect(container.querySelector('.tab-bar-tabs')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Files' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '☰' }))

    for (const label of ['Terminal', 'Terminal 2', 'Terminal 3', 'Files', 'Beads', 'Library', 'Scheduled', 'Settings']) {
      expect(screen.getByRole('button', { name: label })).toBeInTheDocument()
    }
    expect(screen.getByRole('button', { name: 'Terminal' })).toHaveClass('active')

    fireEvent.click(screen.getByRole('button', { name: 'Library' }))

    expect(onTabChange).toHaveBeenCalledWith('library')
  })

  it('opens terminal tab context actions without adding session defaults', () => {
    mockMatchMedia(false)

    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)
    openTabMenu()

    expect(screen.getByRole('menuitem', { name: /Rename tab/i })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Save layout as preset/i })).toBeInTheDocument()
    expect(screen.getByText(/Restore preset/i)).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Clear tab assignments/i })).toBeInTheDocument()

    // Clearing is destructive, so it confirms in the row it was chosen from.
    fireEvent.click(screen.getByRole('menuitem', { name: /Clear tab assignments/i }))
    expect(mockState.clearWorkspaceAssignments).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Confirm clear' }))
    expect(mockState.clearWorkspaceAssignments).toHaveBeenCalledWith('terminal1')
  })

  it('toggles keys from the tab bar and offers the keys panel on its context menu', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} onShowKeys={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Keys on' }))
    expect(mockState.updateSettings).toHaveBeenCalledWith({ keysEnabled: false })
    expect(screen.queryByRole('menuitem', { name: 'Keybindings' })).not.toBeInTheDocument()
  })

  it('renames a tab in the tab, and abandons the rename on Escape', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    openTabMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Rename tab' }))

    // The input takes the tab's own place: nothing floats over the workspace.
    const input = screen.getByRole('textbox', { name: 'Rename Terminal' })
    expect(input).toHaveValue('Terminal')
    fireEvent.change(input, { target: { value: 'Ops' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(mockState.updateSettings).toHaveBeenCalledWith({
      terminalLabels: { ...DEFAULT_SETTINGS.terminalLabels, terminal1: 'Ops' },
    })
    expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument()

    mockState.updateSettings.mockClear()
    openTabMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Rename tab' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Rename Terminal' }), { target: { value: 'Discarded' } })
    fireEvent.keyDown(screen.getByRole('textbox', { name: 'Rename Terminal' }), { key: 'Escape' })

    expect(mockState.updateSettings).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument()
  })

  it('names a preset in the menu and restores one from its submenu', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    openTabMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Save layout as preset' }))

    // The menu becomes the editor rather than handing off to a dialog.
    const input = screen.getByPlaceholderText('Preset name')
    fireEvent.change(input, { target: { value: 'Focus' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(mockState.saveCurrentLayout).toHaveBeenCalledWith('Focus')

    openTabMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Restore preset' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Focus Layout' }))
    expect(mockState.loadPreset).toHaveBeenCalledWith('preset-1')
  })

  // The help view has no tab of its own, so this menu row is the way in to it.
  it('routes to the help view from the keys menu', () => {
    mockMatchMedia(false)
    const onTabChange = vi.fn()
    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} onShowKeys={vi.fn()} />)

    fireEvent.contextMenu(screen.getByRole('button', { name: 'Keys on' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Dashboard Help' }))

    expect(onTabChange).toHaveBeenCalledWith('help')
  })

  it('keeps the keys menu and terminal tab menus mutually exclusive', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} onShowKeys={vi.fn()} />)

    fireEvent.contextMenu(screen.getByRole('button', { name: 'Keys on' }))
    expect(screen.getByRole('menuitem', { name: 'Dashboard Help' })).toBeInTheDocument()

    openTabMenu()
    expect(screen.queryByRole('menuitem', { name: 'Dashboard Help' })).not.toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Rename tab/i })).toBeInTheDocument()
    expect(document.querySelectorAll('.floating-panel-dismiss-layer')).toHaveLength(1)

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('menuitem', { name: /Rename tab/i })).not.toBeInTheDocument()
  })

  // The workspace strip carried these two and a "⋯ Tab" pseudo-tab carried the
  // rest. One menu, on the tab the operator is already looking at, holds them.
  it('carries every moved item on the active tab, and nothing on any other', () => {
    mockMatchMedia(false)
    const { unmount } = render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} onToggleSessionsPinned={vi.fn()} />)

    fireEvent.contextMenu(screen.getByRole('button', { name: 'Terminal' }))

    for (const item of ['Rename tab', 'Save layout as preset', 'Restore preset', 'Clear tab assignments', 'Reconnect frames', 'Claim all', 'Pin sessions panel']) {
      expect(screen.getByRole('menuitem', { name: new RegExp(`^${item}`) })).toBeInTheDocument()
    }
    unmount()

    // No other tab carries it, by caret or by secondary button.
    render(<TabBar activeTab="settings" onTabChange={vi.fn()} />)

    expect(document.querySelector('.tab-menu-caret')).toBeNull()
    fireEvent.contextMenu(screen.getByRole('button', { name: 'Settings' }))
    expect(screen.queryByRole('menuitem', { name: /Rename tab/i })).not.toBeInTheDocument()
  })

  it('reconnects every frame the visible windows hold', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    openTabMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Reconnect frames' }))

    // Window 3 is outside the visible count, so its binding is not a frame.
    expect(reconnect.mock.calls.flat().sort()).toEqual(['alice:alpha', 'bare-session', 'bob:beta'])
  })

  // Claiming resizes a tmux window for everyone watching it, so it acts on the
  // frames in front of this device: the active binding of each visible window,
  // never a session sitting behind a tag or outside the layout.
  it('claims only the sessions the visible windows are showing', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    openTabMenu()
    const claimAll = screen.getByRole('menuitem', { name: /^Claim all/ })
    expect(claimAll).toHaveTextContent('Other devices keep watching')
    fireEvent.click(claimAll)

    expect(claim.mock.calls.flat().sort()).toEqual(['alice:alpha', 'bob:beta'])
  })

  it('refuses a blanket claim on a phone, where one slide is on screen', () => {
    mockMatchMedia(true)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '☰' }))
    fireEvent.click(screen.getByRole('button', { name: 'Terminal tab options' }))
    const claimAll = screen.getByRole('menuitem', { name: /^Claim all/ })
    expect(claimAll).toBeDisabled()
    fireEvent.click(claimAll)

    expect(claim).not.toHaveBeenCalled()
  })

  // Alt+= and Alt+- are the chords for the count; a phone has no keyboard to
  // press them with, so the menu carries the same step on every viewport.
  it('reads the window count in the menu and sets a new one from its submenu', () => {
    mockMatchMedia(false)
    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    openTabMenu()
    const windows = screen.getByRole('menuitem', { name: /^Windows/ })
    expect(windows).toHaveTextContent('2')

    fireEvent.click(windows)
    fireEvent.click(screen.getByRole('menuitem', { name: '3' }))
    expect(mockState.setWindowCount).toHaveBeenCalledWith('terminal1', 3)
  })

  it('reads the Sessions panel pin state in the menu and toggles it there', () => {
    mockMatchMedia(false)
    const onToggleSessionsPinned = vi.fn()
    const { rerender } = render(
      <TabBar activeTab="terminal1" onTabChange={vi.fn()} onToggleSessionsPinned={onToggleSessionsPinned} />,
    )

    openTabMenu()
    expect(screen.getByRole('menuitem', { name: /^Pin sessions panel/ })).toHaveTextContent('off')
    fireEvent.click(screen.getByRole('menuitem', { name: /^Pin sessions panel/ }))
    expect(onToggleSessionsPinned).toHaveBeenCalledTimes(1)

    rerender(
      <TabBar activeTab="terminal1" onTabChange={vi.fn()} sessionsPinned onToggleSessionsPinned={onToggleSessionsPinned} />,
    )
    openTabMenu()
    expect(screen.getByRole('menuitem', { name: /^Pin sessions panel/ })).toHaveTextContent('on')
  })
})
