import { act, fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { resetChordsForTest } from './keys/chords'
import { DEFAULT_SETTINGS, TERMINAL_WORKSPACE_IDS } from './types'
import type { Tab } from './components/TabBar'

const mocks = vi.hoisted(() => ({
  dndProps: null as Record<string, any> | null,
  addSessionToWindow: vi.fn(),
  setWindowCount: vi.fn(),
  removeSessionFromWindow: vi.fn(),
  openSendToSession: vi.fn(),
  windowRevealRequest: null as { workspaceId: string; windowId: string; requestId: number } | null,
  workspaceIds: null as readonly string[] | null,
  settings: null as typeof DEFAULT_SETTINGS | null,
  sessions: [{ name: 'alpha', windows: 1, attached: false, unixUser: 'alice', currentCommand: 'claude' }],
  terminal2WindowCount: 2,
  enabledFlags: [] as string[],
  catalogError: false,
}))

// The canonical four slots every workspace holds; windowCount decides how many
// of them are on screen.
function windowSlots(workspaceId: string) {
  return Array.from({ length: 4 }, (_, index) => ({
    id: `${workspaceId}-window-${index}`,
    boundSessions: [],
    activeSession: null,
    colorIndex: index,
  }))
}

vi.mock('@dnd-kit/core', () => ({
  DndContext: (props: Record<string, any>) => {
    mocks.dndProps = props
    return <>{props.children}</>
  },
  DragOverlay: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div data-testid="drag-overlay-wrapper" className={className}>{children}</div>
  ),
  PointerSensor: function PointerSensor() {},
  useSensor: (...args: unknown[]) => args,
  useSensors: (...args: unknown[]) => args,
}))

vi.mock('./context/SessionContext', () => ({
  SessionProvider: ({ children }: { children: React.ReactNode }) => children,
  useSession: () => ({
    addSessionToWindow: mocks.addSessionToWindow,
    setWindowCount: mocks.setWindowCount,
    removeSessionFromWindow: mocks.removeSessionFromWindow,
    settings: mocks.settings ?? DEFAULT_SETTINGS,
    windowRevealRequest: mocks.windowRevealRequest,
    workspaces: {
      terminal1: { windowCount: 2, windows: windowSlots('terminal1') },
      terminal2: { windowCount: mocks.terminal2WindowCount, windows: windowSlots('terminal2') },
      terminal3: { windowCount: 2, windows: windowSlots('terminal3') },
    },
    workspaceIds: mocks.workspaceIds ?? TERMINAL_WORKSPACE_IDS,
    sessions: mocks.sessions,
    terminalUsers: ['alice', 'build'],
    focusedWindowKey: null,
    openSendToSession: mocks.openSendToSession,
  }),
}))

vi.mock('./components/TabBar', () => ({
  default: ({ onTabChange }: { onTabChange: (tab: Tab) => void }) => (
    <div data-testid="tab-bar">
      {(['terminal1', 'files', 'beads', 'agents', 'library', 'server'] as Tab[]).map(tab => (
        <button key={tab} onClick={() => onTabChange(tab)}>{tab}</button>
      ))}
    </div>
  ),
}))
vi.mock('./components/TerminalWorkspaceDock', () => ({
  default: ({ workspaceId, active }: { workspaceId: string; active?: boolean }) => (
    <div data-workspace={workspaceId} data-active={String(active)}>
      {active && <div className="session-panel" data-active-workspace={workspaceId} />}
      <div data-testid={`${workspaceId} frame`} />
    </div>
  ),
}))
vi.mock('./components/SessionPanel', () => ({
  default: ({ activeWorkspaceId }: { activeWorkspaceId: string }) => <div className="session-panel" data-active-workspace={activeWorkspaceId} />,
}))
vi.mock('./components/TerminalArea', () => ({
  default: ({ workspaceId, active }: { workspaceId: string; active?: boolean }) => (
    <div data-workspace={workspaceId} data-active={String(active)}><div data-testid={`${workspaceId} frame`} /></div>
  ),
}))
vi.mock('./components/FilesView', () => ({ default: function FilesProbe() {
  const [draft, setDraft] = useState('')
  return <input aria-label="Files draft" value={draft} onChange={event => setDraft(event.target.value)} />
} }))
vi.mock('./components/SettingsView', () => ({ default: () => null }))
vi.mock('./components/Peek', () => ({ default: () => null }))
vi.mock('./components/ImageGlance', () => ({ default: () => null }))
vi.mock('./components/SendDrawer', () => ({ default: () => null }))
vi.mock('./components/HelpView', () => ({ default: () => null }))
vi.mock('./beads/BeadCatalog', () => ({ default: () => {
  if (mocks.catalogError) throw new Error('Catalog unavailable')
  return null
} }))
vi.mock('./components/BeadsView', () => ({ default: () => <div data-testid="beads-view" /> }))
vi.mock('./components/BeadsColumn', () => ({ default: () => <div data-testid="beads-column" /> }))
vi.mock('./components/AgentsView', () => ({ default: () => <div data-testid="agents-view" /> }))
vi.mock('./components/LibraryView', () => ({ default: () => <div data-testid="library-view" /> }))
vi.mock('./components/SystemStatusView', () => ({
  default: ({ active }: { active?: boolean }) => <div data-testid="system-status-view" data-active={String(active)} />,
}))
vi.mock('./components/ScheduledTasksView', () => ({ default: () => null }))
vi.mock('./keys/KeysPanel', () => ({
  default: ({ isOpen }: { isOpen: boolean }) => (isOpen ? <div data-testid="keys-panel" /> : null),
}))
vi.mock('./keys/LeaderStrip', () => ({ default: () => null }))
vi.mock('./components/LayoutPresetsPanel', () => ({ default: () => null }))
vi.mock('./components/TerminalPool', () => ({
  TerminalPoolProvider: ({ children }: { children: React.ReactNode }) => children,
  useTerminalPool: () => ({ terminals: new Map(), connectionStates: new Map() }),
}))
vi.mock('./hooks/useKeyboardShortcuts', () => ({ useKeyboardShortcuts: vi.fn() }))
vi.mock('./featureFlags', () => ({
  installFeatureFlagHelpers: vi.fn(),
  isFeatureEnabled: (flag: string) => mocks.enabledFlags.includes(flag),
}))

const panelDrag = {
  active: {
    id: 'alice:alpha',
    data: { current: { type: 'session', sessionName: 'alpha', sessionKey: 'alice:alpha', unixUser: 'alice' } },
  },
}

const tagDrag = {
  active: {
    id: 'tag-terminal1-terminal1-window-0-alice:alpha',
    data: {
      current: {
        type: 'tag',
        sessionName: 'alpha',
        sessionKey: 'alice:alpha',
        unixUser: 'alice',
        sourceWorkspaceId: 'terminal1',
        sourceWindowId: 'terminal1-window-0',
      },
    },
  },
}

describe('App drag lifecycle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.dndProps = null
    mocks.windowRevealRequest = null
    mocks.workspaceIds = null
    mocks.settings = null
    mocks.terminal2WindowCount = 2
    mocks.enabledFlags = []
  })

  it('uses one reset path for drag cancel and clears all active drag visuals', () => {
    const { container } = render(<App />)

    act(() => mocks.dndProps?.onDragStart(panelDrag))

    expect(container.querySelector('.dashboard')).toHaveClass('is-dragging')
    expect(screen.getByText('alpha')).toBeInTheDocument()

    act(() => mocks.dndProps?.onDragCancel())

    expect(container.querySelector('.dashboard')).not.toHaveClass('is-dragging')
    expect(screen.queryByText('alpha')).not.toBeInTheDocument()
  })

  it('treats tag drops on off-target surfaces as no-ops and still resets drag state', () => {
    const { container } = render(<App />)

    act(() => mocks.dndProps?.onDragStart(tagDrag))
    act(() => mocks.dndProps?.onDragEnd({ ...tagDrag, over: null }))

    expect(mocks.removeSessionFromWindow).not.toHaveBeenCalled()
    expect(mocks.addSessionToWindow).not.toHaveBeenCalled()
    expect(container.querySelector('.dashboard')).not.toHaveClass('is-dragging')
    expect(screen.queryByText('alpha')).not.toBeInTheDocument()
  })

  it('renders one faithful tag overlay with its grip and qualified Unix user visual', () => {
    const { container } = render(<App />)

    act(() => mocks.dndProps?.onDragStart(tagDrag))

    const overlays = container.querySelectorAll('.dragging-overlay')
    expect(overlays).toHaveLength(1)
    expect(overlays[0]).toHaveClass('session-tag')
    expect(overlays[0].querySelector('.session-user-badge')).toHaveTextContent('A')
    expect(overlays[0].querySelector('.session-user-badge')).toHaveAttribute('title', 'Unix user: alice')
    // The ghost is made of the same pieces as the tag it left: badge, mark, name.
    expect(overlays[0].querySelector('[data-harness="claude-code"]')).not.toBeNull()
    expect(overlays[0].querySelector('.session-label')).toHaveTextContent('alpha')
  })

  // The seam between two tiles is the layout's own control: dropping there
  // makes the window the session needs and binds it in one gesture.
  it('adds a window and binds the session when a drop lands in the seam between tiles', () => {
    render(<App />)

    act(() => mocks.dndProps?.onDragStart(panelDrag))
    act(() => mocks.dndProps?.onDragEnd({
      ...panelDrag,
      over: { data: { current: { type: 'window-gap', workspaceId: 'terminal2' } } },
    }))

    expect(mocks.setWindowCount).toHaveBeenCalledWith('terminal2', 3)
    expect(mocks.addSessionToWindow).toHaveBeenCalledWith('terminal2', 'terminal2-window-2', 'alpha', 'alice')
  })

  it('adds nothing when the layout is already at its four windows', () => {
    mocks.terminal2WindowCount = 4
    render(<App />)

    act(() => mocks.dndProps?.onDragStart(panelDrag))
    act(() => mocks.dndProps?.onDragEnd({
      ...panelDrag,
      over: { data: { current: { type: 'window-gap', workspaceId: 'terminal2' } } },
    }))

    expect(mocks.setWindowCount).not.toHaveBeenCalled()
    expect(mocks.addSessionToWindow).not.toHaveBeenCalled()
  })

  it('moves tags only when dropped on a different explicit window target', () => {
    render(<App />)

    act(() => mocks.dndProps?.onDragStart(tagDrag))
    act(() => mocks.dndProps?.onDragEnd({
      ...tagDrag,
      over: { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal2-window-1' } } },
    }))

    expect(mocks.addSessionToWindow).toHaveBeenCalledWith('terminal2', 'terminal2-window-1', 'alpha', 'alice')
    expect(mocks.removeSessionFromWindow).not.toHaveBeenCalled()
  })

  it('marks only the active terminal workspace as active for drop feedback', () => {
    const { container } = render(<App />)

    expect(container.querySelector('[data-workspace="terminal1"]')).toHaveAttribute('data-active', 'true')
    expect(container.querySelector('[data-workspace="terminal2"]')).toHaveAttribute('data-active', 'false')
    expect(container.querySelector('[data-workspace="terminal3"]')).toHaveAttribute('data-active', 'false')
    expect(container.querySelector('.session-panel')).toHaveAttribute('data-active-workspace', 'terminal1')
  })

  it('applies restored appearance settings to the document', () => {
    mocks.settings = { ...DEFAULT_SETTINGS, fontSize: 18 }
    const { rerender } = render(<App />)

    expect(document.documentElement.style.getPropertyValue('--terminal-font-size')).toBe('18px')

    mocks.settings = { ...DEFAULT_SETTINGS, fontSize: 16 }
    rerender(<App />)
    expect(document.documentElement.style.getPropertyValue('--terminal-font-size')).toBe('16px')
  })

  it('switches to the workspace requested by assigned-session navigation', () => {
    mocks.windowRevealRequest = { workspaceId: 'terminal3', windowId: 'terminal3-window-2', requestId: 1 }
    const { container } = render(<App />)

    expect(container.querySelector('[data-workspace="terminal3"]')).toHaveAttribute('data-active', 'true')
    expect(container.querySelector('.session-panel')).toHaveAttribute('data-active-workspace', 'terminal3')
  })

  it('keeps docks mounted for workspaces hidden by a shrunken tab count', () => {
    mocks.workspaceIds = ['terminal1', 'terminal2']
    const { container } = render(<App />)

    const hiddenDock = container.querySelector('[data-workspace="terminal3"]')
    expect(hiddenDock).not.toBeNull()
    expect(hiddenDock).toHaveAttribute('data-active', 'false')
  })

  it('falls back to terminal1 when the active workspace tab becomes hidden', () => {
    mocks.workspaceIds = ['terminal1', 'terminal2']
    mocks.windowRevealRequest = { workspaceId: 'terminal3', windowId: 'terminal3-window-2', requestId: 1 }
    const { container } = render(<App />)

    expect(container.querySelector('[data-workspace="terminal3"]')).toHaveAttribute('data-active', 'false')
    expect(container.querySelector('[data-workspace="terminal1"]')).toHaveAttribute('data-active', 'true')
  })
})

// The backend owns Server history. Optional views begin on first use, then
// remain mounted to preserve the operator's state while another tab is active.
describe('App optional view activation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.dndProps = null
    mocks.windowRevealRequest = null
    mocks.workspaceIds = null
    mocks.settings = null
    mocks.terminal2WindowCount = 2
    mocks.enabledFlags = ['serverStatusTab']
  })

  it('defers unused views and retains a visited Files draft across tab switches', async () => {
    render(<App />)
    await act(async () => {})

    expect(screen.queryByLabelText('Files draft')).not.toBeInTheDocument()
    for (const id of ['beads-view', 'agents-view', 'library-view', 'system-status-view', 'beads-column']) {
      expect(screen.queryByTestId(id)).not.toBeInTheDocument()
    }

    fireEvent.click(screen.getByRole('button', { name: 'files' }))
    const draft = await screen.findByLabelText('Files draft')
    fireEvent.change(draft, { target: { value: 'unsaved notes' } })
    fireEvent.click(screen.getByRole('button', { name: 'agents' }))
    await screen.findByTestId('agents-view')
    expect(draft).not.toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'files' }))
    expect(screen.getByLabelText('Files draft')).toBe(draft)
    expect(draft).toHaveValue('unsaved notes')
  })

  it('keeps the visited Server status view mounted and inactive behind another tab', async () => {
    render(<App />)
    expect(screen.queryByTestId('system-status-view')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'server' }))

    const statusView = await screen.findByTestId('system-status-view')
    expect(statusView).toHaveAttribute('data-active', 'true')
    fireEvent.click(screen.getByRole('button', { name: 'terminal1' }))
    expect(screen.getByTestId('system-status-view')).toBe(statusView)
    expect(statusView).toHaveAttribute('data-active', 'false')
    expect(statusView.parentElement).toHaveStyle({ display: 'none' })
  })

  it('contains a catalog render failure without losing terminal workspaces', () => {
    const log = vi.spyOn(console, 'error').mockImplementation(() => {})
    mocks.catalogError = true
    try {
      render(<App />)
      expect(screen.getByText('Catalog unavailable')).toBeInTheDocument()
      expect(screen.getByTestId('terminal1 frame')).toBeInTheDocument()
    } finally {
      mocks.catalogError = false
      log.mockRestore()
    }
  })
})

// The keys panel is a glance, so the two ways in are also the ways out: the
// leader and Alt+K each toggle it.
describe('App keys panel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.dndProps = null
    mocks.windowRevealRequest = null
    mocks.workspaceIds = null
    mocks.settings = null
    mocks.terminal2WindowCount = 2
    mocks.enabledFlags = []
    resetChordsForTest()
  })

  const key = (init: KeyboardEventInit) => {
    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init }))
    })
  }
  const leader = () => key({ key: ' ', code: 'Space', ctrlKey: true, shiftKey: true })
  const altK = () => key({ key: 'k', altKey: true })

  it('toggles on the leader and on Alt+K', () => {
    render(<App />)
    expect(screen.queryByTestId('keys-panel')).toBeNull()

    leader()
    expect(screen.getByTestId('keys-panel')).toBeInTheDocument()
    leader()
    expect(screen.queryByTestId('keys-panel')).toBeNull()

    altK()
    expect(screen.getByTestId('keys-panel')).toBeInTheDocument()
    altK()
    expect(screen.queryByTestId('keys-panel')).toBeNull()
  })
})
