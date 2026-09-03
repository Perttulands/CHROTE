import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { DEFAULT_SETTINGS, TERMINAL_WORKSPACE_IDS } from './types'

const mocks = vi.hoisted(() => ({
  dndProps: null as Record<string, any> | null,
  addSessionToWindow: vi.fn(),
  removeSessionFromWindow: vi.fn(),
  openSendToSession: vi.fn(),
  windowRevealRequest: null as { workspaceId: string; windowId: string; requestId: number } | null,
  workspaceIds: null as readonly string[] | null,
  settings: null as typeof DEFAULT_SETTINGS | null,
  sessions: [{ name: 'alpha', windows: 1, attached: false, unixUser: 'alice', currentCommand: 'claude' }],
}))

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
    removeSessionFromWindow: mocks.removeSessionFromWindow,
    settings: mocks.settings ?? DEFAULT_SETTINGS,
    windowRevealRequest: mocks.windowRevealRequest,
    workspaces: {
      terminal1: { windows: [] },
      terminal2: { windows: [] },
      terminal3: { windows: [] },
    },
    workspaceIds: mocks.workspaceIds ?? TERMINAL_WORKSPACE_IDS,
    sessions: mocks.sessions,
    terminalUsers: ['alice', 'build'],
    focusedWindowKey: null,
    openSendToSession: mocks.openSendToSession,
  }),
}))

vi.mock('./components/TabBar', () => ({ default: () => <div data-testid="tab-bar" /> }))
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
vi.mock('./components/FilesView', () => ({ default: () => null }))
vi.mock('./components/SettingsView', () => ({ default: () => null }))
vi.mock('./components/FloatingModal', () => ({ default: () => null }))
vi.mock('./components/SendToSessionModal', () => ({ default: () => null }))
vi.mock('./components/HelpView', () => ({ default: () => null }))
vi.mock('./components/BeadsView', () => ({ default: () => null }))
vi.mock('./components/ServicesView', () => ({ default: () => null }))
vi.mock('./components/SystemStatusView', () => ({ default: () => null }))
vi.mock('./components/ScheduledTasksView', () => ({ default: () => null }))
vi.mock('./components/ErrorBoundary', () => ({ default: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('./components/ToastNotification', () => ({ ToastContainer: () => null }))
vi.mock('./keys/KeysPanel', () => ({ default: () => null }))
vi.mock('./keys/LeaderStrip', () => ({ default: () => null }))
vi.mock('./components/LayoutPresetsPanel', () => ({ default: () => null }))
vi.mock('./components/TerminalPool', () => ({ TerminalPoolProvider: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('./hooks/useKeyboardShortcuts', () => ({ useKeyboardShortcuts: vi.fn() }))
vi.mock('./featureFlags', () => ({ installFeatureFlagHelpers: vi.fn(), isFeatureEnabled: () => false }))

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
  })

  it('uses one reset path for drag cancel and clears all active drag visuals', () => {
    const { container } = render(<App />)

    act(() => mocks.dndProps?.onDragStart(panelDrag))

    expect(container.querySelector('.dashboard')).toHaveClass('is-dragging')
    expect(screen.getByTestId('drag-overlay-wrapper')).toHaveClass('drag-overlay-wrapper')
    expect(screen.getByText('alpha')).toBeInTheDocument()

    act(() => mocks.dndProps?.onDragCancel())

    expect(container.querySelector('.dashboard')).not.toHaveClass('is-dragging')
    expect(screen.queryByText('alpha')).not.toBeInTheDocument()
  })

  it('registers only pointer dragging with an 8px activation threshold', () => {
    render(<App />)

    expect(mocks.dndProps?.sensors).toHaveLength(1)
    expect(mocks.dndProps?.sensors[0][0].name).toBe('PointerSensor')
    expect(mocks.dndProps?.sensors[0][1]).toEqual({ activationConstraint: { distance: 8 } })
  })

  it.each([
    ['unknown type', { active: { id: 'alice:alpha', data: { current: { ...panelDrag.active.data.current, type: 'unknown' } } } }],
    ['missing session name', { active: { id: 'alice:alpha', data: { current: { ...panelDrag.active.data.current, sessionName: '' } } } }],
    ['missing session key', { active: { id: 'alice:alpha', data: { current: { ...panelDrag.active.data.current, sessionKey: '   ' } } } }],
    ['non-string Unix user', { active: { id: 'alice:alpha', data: { current: { ...panelDrag.active.data.current, unixUser: 42 } } } }],
    ['invalid tag source workspace', { active: { ...tagDrag.active, data: { current: { ...tagDrag.active.data.current, sourceWorkspaceId: 'files' } } } }],
    ['invalid tag source window', { active: { ...tagDrag.active, data: { current: { ...tagDrag.active.data.current, sourceWindowId: 'terminal2-window-0' } } } }],
    ['leading-zero tag source window 00', { active: { ...tagDrag.active, data: { current: { ...tagDrag.active.data.current, sourceWindowId: 'terminal1-window-00' } } } }],
    ['leading-zero tag source window 01', { active: { ...tagDrag.active, data: { current: { ...tagDrag.active.data.current, sourceWindowId: 'terminal1-window-01' } } } }],
    ['padded tag source window 0003', { active: { ...tagDrag.active, data: { current: { ...tagDrag.active.data.current, sourceWindowId: 'terminal1-window-0003' } } } }],
  ])('rejects malformed drag starts: %s', (_label, malformedDrag) => {
    const { container } = render(<App />)

    act(() => mocks.dndProps?.onDragStart(malformedDrag))

    expect(container.querySelector('.dashboard')).not.toHaveClass('is-dragging')
    expect(container.querySelector('.dragging-overlay')).toBeNull()
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
    expect(overlays[0].querySelector('.drag-overlay-grip')).toBeNull()
    expect(overlays[0].querySelector('.session-user-badge')).toHaveTextContent('A')
    expect(overlays[0].querySelector('.session-user-badge')).toHaveAttribute('title', 'Unix user: alice')
    // The ghost is made of the same pieces as the tag it left: badge, mark, name.
    expect(overlays[0].querySelector('[data-harness="claude-code"]')).not.toBeNull()
    expect(overlays[0].querySelector('.session-label')).toHaveTextContent('alpha')
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

  it.each([
    ['unknown active type', { ...panelDrag, active: { ...panelDrag.active, data: { current: { ...panelDrag.active.data.current, type: 'unknown' } } } }, { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal2-window-1' } } }],
    ['missing session name', { ...panelDrag, active: { ...panelDrag.active, data: { current: { ...panelDrag.active.data.current, sessionName: '' } } } }, { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal2-window-1' } } }],
    ['missing session key', { ...panelDrag, active: { ...panelDrag.active, data: { current: { ...panelDrag.active.data.current, sessionKey: '' } } } }, { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal2-window-1' } } }],
    ['invalid Unix user', { ...panelDrag, active: { ...panelDrag.active, data: { current: { ...panelDrag.active.data.current, unixUser: 42 } } } }, { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal2-window-1' } } }],
    ['invalid target type', panelDrag, { data: { current: { type: 'header', workspaceId: 'terminal2', windowId: 'terminal2-window-1' } } }],
    ['invalid target workspace', panelDrag, { data: { current: { type: 'window', workspaceId: 'files', windowId: 'files-window-1' } } }],
    ['mismatched target window', panelDrag, { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal1-window-1' } } }],
    ['out-of-range target window', panelDrag, { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal2-window-9' } } }],
    ['leading-zero target window 00', panelDrag, { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal2-window-00' } } }],
    ['leading-zero target window 01', panelDrag, { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal2-window-01' } } }],
    ['padded target window 0003', panelDrag, { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal2-window-0003' } } }],
    ['invalid tag source IDs', { ...tagDrag, active: { ...tagDrag.active, data: { current: { ...tagDrag.active.data.current, sourceWindowId: 'terminal3-window-0' } } } }, { data: { current: { type: 'window', workspaceId: 'terminal2', windowId: 'terminal2-window-1' } } }],
  ])('does not mutate for malformed drag ends: %s', (_label, malformedDrag, over) => {
    const { container } = render(<App />)

    act(() => mocks.dndProps?.onDragStart(panelDrag))
    act(() => mocks.dndProps?.onDragEnd({ ...malformedDrag, over }))

    expect(mocks.addSessionToWindow).not.toHaveBeenCalled()
    expect(container.querySelector('.dashboard')).not.toHaveClass('is-dragging')
    expect(container.querySelector('.dragging-overlay')).toBeNull()
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
