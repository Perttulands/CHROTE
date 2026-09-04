import { Suspense, lazy, useCallback, useEffect, useMemo, useState } from 'react'
import { DndContext, DragEndEvent, DragStartEvent, DragOverlay, useSensor, useSensors, PointerSensor } from '@dnd-kit/core'
import './App.css'
import { SessionProvider, useSession } from './context/SessionContext'
import { StatusProvider } from './context/StatusContext'
import { TableProvider } from './context/TableContext'
import TabBar, { Tab } from './components/TabBar'
import TerminalWorkspaceDock from './components/TerminalWorkspaceDock'
import Peek from './components/Peek'
import ImageGlance from './components/ImageGlance'
import SendDrawer from './components/SendDrawer'
import type { BeadsRevealRequest } from './components/BeadsView'
import ErrorBoundary from './components/ErrorBoundary'
import Skeleton from './components/LoadingSkeleton'
import StatusLine from './components/StatusLine'
import Toast from './components/Toast'
import KeysPanel from './keys/KeysPanel'
import KeyEcho from './keys/KeyEcho'
import DevMode from './dev/DevMode'
import { closeLeaderWindow, useLeader } from './keys/chords'
import { TerminalPoolProvider } from './components/TerminalPool'
import { useOpenInFilesRequest } from './terminal/openInFiles'
import { SessionCommandMark, SessionLabel } from './components/sessionLabel'
import {
  readSessionsDockState,
  writeSessionsDockState,
  type SessionsDockState,
} from './components/workspaceFilesState'
import { useKeyboardShortcuts } from './hooks/useKeyboardShortcuts'
import { useAppChords } from './keys/useAppChords'
import { installFeatureFlagHelpers, isFeatureEnabled } from './featureFlags'
import { getSessionNameFromKey, getTerminalUserInitial, isTerminalWorkspaceId, sortTerminalWorkspaceIds } from './types'
import type { WorkspaceId } from './types'
import { ThemeProvider, useTheme } from './theme/ThemeContext'
import { identityColorFor } from './theme/theme'

// Non-terminal views load as route-level chunks on first visit. The terminal
// docks and everything they depend on stay eager: they are the startup surface
// and their pooled terminals must never be interrupted by a chunk load. Each lazy
// view gets its OWN Suspense boundary inside its mount branch — one shared
// boundary around .dashboard-content would swap the whole tree (terminal docks
// included) for a fallback while a chunk loads, killing live terminals.
const FilesView = lazy(() => import('./components/FilesView'))
const SettingsView = lazy(() => import('./components/SettingsView'))
const HelpView = lazy(() => import('./components/HelpView'))
const BeadsView = lazy(() => import('./components/BeadsView'))
const LibraryView = lazy(() => import('./components/LibraryView'))
const AgentsView = lazy(() => import('./components/AgentsView'))
const SystemStatusView = lazy(() => import('./components/SystemStatusView'))
const ScheduledTasksView = lazy(() => import('./components/ScheduledTasksView'))

function ViewFallback() {
  return <div className="view-chunk-loading"><Skeleton height="14px" width="180px" /></div>
}

interface ActiveDrag {
  name: string
  type: 'session' | 'tag'
  unixUser?: string
}

interface SessionDragData {
  type: 'session'
  sessionName: string
  sessionKey: string
  unixUser?: string
}

interface TagDragData {
  type: 'tag'
  sessionName: string
  sessionKey: string
  unixUser?: string
  sourceWorkspaceId: WorkspaceId
  sourceWindowId: string
}

interface WindowDropData {
  type: 'window'
  workspaceId: WorkspaceId
  windowId: string
}

interface WindowGapDropData {
  type: 'window-gap'
  workspaceId: WorkspaceId
}

/** The layout never grows past the four windows the grid classes describe. */
const MAX_WINDOWS = 4

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0
}

function isWindowIdForWorkspace(value: unknown, workspaceId: WorkspaceId): value is string {
  if (typeof value !== 'string') return false
  return /^[0-3]$/.test(value.slice(`${workspaceId}-window-`.length)) &&
    value.startsWith(`${workspaceId}-window-`)
}

function readDragData(value: unknown): SessionDragData | TagDragData | null {
  if (!value || typeof value !== 'object') return null
  const data = value as Record<string, unknown>
  if (!isNonEmptyString(data.sessionName) || !isNonEmptyString(data.sessionKey)) return null
  if (data.unixUser !== undefined && typeof data.unixUser !== 'string') return null

  if (data.type === 'session') {
    return {
      type: 'session',
      sessionName: data.sessionName,
      sessionKey: data.sessionKey,
      ...(data.unixUser !== undefined ? { unixUser: data.unixUser } : {}),
    }
  }

  if (
    data.type === 'tag' &&
    isTerminalWorkspaceId(data.sourceWorkspaceId) &&
    isWindowIdForWorkspace(data.sourceWindowId, data.sourceWorkspaceId)
  ) {
    return {
      type: 'tag',
      sessionName: data.sessionName,
      sessionKey: data.sessionKey,
      sourceWorkspaceId: data.sourceWorkspaceId,
      sourceWindowId: data.sourceWindowId,
      ...(data.unixUser !== undefined ? { unixUser: data.unixUser } : {}),
    }
  }

  return null
}

function readWindowDropData(value: unknown): WindowDropData | null {
  if (!value || typeof value !== 'object') return null
  const data = value as Record<string, unknown>
  if (
    data.type !== 'window' ||
    !isTerminalWorkspaceId(data.workspaceId) ||
    !isWindowIdForWorkspace(data.windowId, data.workspaceId)
  ) return null

  return { type: 'window', workspaceId: data.workspaceId, windowId: data.windowId }
}

function readWindowGapDropData(value: unknown): WindowGapDropData | null {
  if (!value || typeof value !== 'object') return null
  const data = value as Record<string, unknown>
  if (data.type !== 'window-gap' || !isTerminalWorkspaceId(data.workspaceId)) return null

  return { type: 'window-gap', workspaceId: data.workspaceId }
}

// Dragged item overlay component. It is drawn from the same pieces as the tag
// and the row it was picked up from, so the ghost is the thing itself.
function DraggedSessionOverlay({ drag }: { drag: ActiveDrag }) {
  const { sessions, terminalUsers } = useSession()
  const theme = useTheme()
  const { name, type, unixUser } = drag
  const displayName = getSessionNameFromKey(name)
  const badgeClassName = type === 'tag' ? 'session-user-badge' : 'unix-user-badge'
  const dragged = sessions.find(session => session.name === displayName
    && (!unixUser || session.unixUser === unixUser))
  return (
    <div className={`${type === 'tag' ? 'session-tag' : 'session-item'} dragging-overlay`}>
      {unixUser && (
        <span
          className={badgeClassName}
          style={{ backgroundColor: identityColorFor(unixUser, terminalUsers, theme) }}
          title={`Unix user: ${unixUser}`}
          aria-label={`Unix user ${unixUser}`}
        >
          {getTerminalUserInitial(unixUser)}
        </span>
      )}
      <SessionCommandMark command={dragged?.currentCommand} />
      <SessionLabel name={displayName} className={type === 'tag' ? 'tag-name' : 'session-name'} />
    </div>
  )
}

function DashboardContent() {
  const [activeTab, setActiveTab] = useState<Tab>('terminal1')
  const [lastActiveWorkspaceId, setLastActiveWorkspaceId] = useState<WorkspaceId>('terminal1')
  const [sessionsDockState, setSessionsDockState] = useState<SessionsDockState>(readSessionsDockState)
  const [openFilesWorkspaceIds, setOpenFilesWorkspaceIds] = useState<Set<WorkspaceId>>(() => new Set())
  const [activeDrag, setActiveDrag] = useState<ActiveDrag | null>(null)
  const [keysPanelOpen, setKeysPanelOpen] = useState(false)
  const [filesNavigateRequest, setFilesNavigateRequest] = useState<{ path: string; nonce: number } | null>(null)
  const [beadsRevealRequest, setBeadsRevealRequest] = useState<BeadsRevealRequest | null>(null)
  const {
    addSessionToWindow,
    setWindowCount,
    settings,
    windowRevealRequest,
    workspaces,
    workspaceIds,
    focusedWindowKey,
    openSendToSession,
  } = useSession()
  const filesSendTarget = useMemo(() => {
    if (!focusedWindowKey) return null
    for (const [workspaceId, workspace] of Object.entries(workspaces)) {
      const terminalWindow = workspace.windows.find(window => `${workspaceId}-${window.id}` === focusedWindowKey)
      if (terminalWindow?.activeSession) return terminalWindow.activeSession
    }
    return null
  }, [focusedWindowKey, workspaces])
  const handleSendFilePath = useCallback((path: string) => {
    if (filesSendTarget) openSendToSession({ targetSessionKey: filesSendTarget, reference: `path ${path}` })
  }, [filesSendTarget, openSendToSession])
  const persistFilesTabState = isFeatureEnabled('filesPersistTabState')
  const serverStatusTab = isFeatureEnabled('serverStatusTab')
  const handleFilesOpenChange = useCallback((workspaceId: WorkspaceId, open: boolean) => {
    setOpenFilesWorkspaceIds(previous => {
      if (previous.has(workspaceId) === open) return previous
      const next = new Set(previous)
      if (open) next.add(workspaceId)
      else next.delete(workspaceId)
      return next
    })
  }, [])
  const sessionsForcedPinned = openFilesWorkspaceIds.size > 0

  useEffect(() => {
    writeSessionsDockState(sessionsDockState)
  }, [sessionsDockState])

  // Every workspace in state keeps its dock mounted — including ones hidden by
  // a shrunken tab count — so panel state and pooled terminals survive.
  const mountedWorkspaceIds = useMemo(
    () => sortTerminalWorkspaceIds(Object.keys(workspaces) as WorkspaceId[]),
    [workspaces],
  )

  // A tab-count shrink can hide the active workspace; fall back to terminal1.
  useEffect(() => {
    if (isTerminalWorkspaceId(activeTab, mountedWorkspaceIds) && !workspaceIds.includes(activeTab)) {
      setActiveTab('terminal1')
      setLastActiveWorkspaceId('terminal1')
    }
  }, [activeTab, workspaceIds, mountedWorkspaceIds])

  useEffect(() => {
    if (!workspaceIds.includes(lastActiveWorkspaceId)) setLastActiveWorkspaceId('terminal1')
  }, [lastActiveWorkspaceId, workspaceIds])

  // The panel is a glance, so the chord that opens it closes it as well.
  const toggleKeysPanel = useCallback(() => setKeysPanelOpen(open => !open), [])
  const handleCloseKeys = useCallback(() => setKeysPanelOpen(false), [])
  const toggleSessionsPanel = useCallback(() => {
    setSessionsDockState(previous => ({ ...previous, open: !previous.open }))
  }, [])
  const openSessionsPanel = useCallback(() => {
    setSessionsDockState(previous => previous.open ? previous : { ...previous, open: true })
  }, [])
  const toggleSessionsPinned = useCallback(() => {
    setSessionsDockState(previous => ({ ...previous, pinned: !previous.pinned }))
  }, [])
  const handleTabChange = useCallback((tab: Tab) => {
    setActiveTab(tab)
    if (isTerminalWorkspaceId(tab, mountedWorkspaceIds)) setLastActiveWorkspaceId(tab)
  }, [mountedWorkspaceIds])
  // A Bead read in the card is a Bead in a project: Open in Beads puts the tab
  // on that project with the id already searched for.
  const handleOpenInBeads = useCallback((projectPath: string, id: string) => {
    setBeadsRevealRequest({ projectPath, id, nonce: Date.now() })
    setActiveTab('beads')
  }, [])
  const handleOpenProjectInFiles = useCallback((path: string) => {
    setFilesNavigateRequest({ path, nonce: Date.now() })
    setActiveTab('files')
  }, [])
  useEffect(() => {
    if (windowRevealRequest) {
      setActiveTab(windowRevealRequest.workspaceId)
      setLastActiveWorkspaceId(windowRevealRequest.workspaceId)
    }
  }, [windowRevealRequest])

  // A path activated in a terminal goes to the Files panel of the terminal
  // tab the operator is on; on any other tab, to the Files tab.
  const openInFilesRequest = useOpenInFilesRequest()
  const openInFilesOnTerminalTab = isTerminalWorkspaceId(activeTab, mountedWorkspaceIds)
  useEffect(() => {
    if (!openInFilesRequest || openInFilesOnTerminalTab) return
    handleOpenProjectInFiles(openInFilesRequest.path)
    // Each request is answered once, where the operator was when it was made.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [openInFilesRequest])

  // The leader is discovery: it toggles the keys panel and shuts its own
  // window, because from here the next key is search text rather than a chord.
  const { leaderOpen } = useLeader()
  useEffect(() => {
    if (!leaderOpen) return
    setKeysPanelOpen(open => !open)
    closeLeaderWindow()
  }, [leaderOpen])

  // Plain keys outside a terminal; the leader model lives in the registry.
  useKeyboardShortcuts({
    onShowKeys: toggleKeysPanel,
    isKeysPanelOpen: keysPanelOpen,
  })

  useAppChords({
    activeTab,
    onTabChange: handleTabChange,
    onToggleSessionsPanel: toggleSessionsPanel,
    onOpenSessionsPanel: openSessionsPanel,
    onToggleKeysPanel: toggleKeysPanel,
  })

  useEffect(() => {
    installFeatureFlagHelpers()
  }, [])

  // Apply font size as CSS variable for terminal styling
  useEffect(() => {
    document.documentElement.style.setProperty('--terminal-font-size', `${settings.fontSize}px`)
  }, [settings.fontSize])

  const resetDrag = useCallback(() => {
    setActiveDrag(null)
  }, [])

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
  )

  const handleDragStart = (event: DragStartEvent) => {
    const data = readDragData(event.active.data.current)
    if (!data) {
      resetDrag()
      return
    }

    setActiveDrag({ name: data.sessionKey, type: data.type, unixUser: data.unixUser })
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    resetDrag()

    if (!over) {
      // Off-target drops never mutate assignment. Detaching is explicit via
      // the tag remove button or the session context menu's Unassign action.
      return
    }

    const dragData = readDragData(active.data.current)
    if (!dragData) return

    // The seam between tiles is a window that does not exist yet: the drop
    // makes it and lands the session in it, in that order.
    const gapData = readWindowGapDropData(over.data.current)
    if (gapData) {
      const workspace = workspaces[gapData.workspaceId]
      const added = workspace?.windows[workspace.windowCount]
      if (!workspace || !added || workspace.windowCount >= MAX_WINDOWS) return
      setWindowCount(gapData.workspaceId, workspace.windowCount + 1)
      addSessionToWindow(gapData.workspaceId, added.id, dragData.sessionName, dragData.unixUser)
      return
    }

    const targetData = readWindowDropData(over.data.current)
    if (!targetData) return

    if (dragData.type === 'session') {
      addSessionToWindow(targetData.workspaceId, targetData.windowId, dragData.sessionName, dragData.unixUser)
    } else if (
      dragData.sourceWindowId !== targetData.windowId ||
      dragData.sourceWorkspaceId !== targetData.workspaceId
    ) {
      addSessionToWindow(targetData.workspaceId, targetData.windowId, dragData.sessionName, dragData.unixUser)
    }
  }

  return (
    <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd} onDragCancel={resetDrag}>
      <TableProvider openInBeads={handleOpenInBeads}>
      <div className={`dashboard ${activeDrag ? 'is-dragging' : ''}`}>
        <TabBar
          activeTab={activeTab}
          onTabChange={handleTabChange}
          onShowKeys={toggleKeysPanel}
          sessionsPinned={sessionsDockState.pinned}
          onToggleSessionsPinned={toggleSessionsPinned}
        />

        <div className="dashboard-content">
          {/* Terminal workspaces stay mounted so panel state and pooled terminal connections survive tab switches. */}
          {mountedWorkspaceIds.map(workspaceId => (
            <TerminalWorkspaceDock
              key={workspaceId}
              workspaceId={workspaceId}
              active={activeTab === workspaceId}
              sessionsDockState={sessionsDockState}
              onSessionsDockStateChange={setSessionsDockState}
              sessionsForcedPinned={sessionsForcedPinned}
              onFilesOpenChange={handleFilesOpenChange}
              onOpenInFiles={handleOpenProjectInFiles}
              openFilesRequest={activeTab === workspaceId ? openInFilesRequest : null}
            />
          ))}
          {persistFilesTabState ? (
            <div style={{ display: activeTab === 'files' ? 'contents' : 'none' }}>
              <ErrorBoundary>
                <Suspense fallback={<ViewFallback />}>
                  <FilesView
                    navigateRequest={filesNavigateRequest}
                    onSendPath={filesSendTarget ? handleSendFilePath : undefined}
                    sendTargetLabel={filesSendTarget ? getSessionNameFromKey(filesSendTarget) : null}
                  />
                </Suspense>
              </ErrorBoundary>
            </div>
          ) : (
            activeTab === 'files' && (
              <ErrorBoundary>
                <Suspense fallback={<ViewFallback />}>
                  <FilesView
                    navigateRequest={filesNavigateRequest}
                    onSendPath={filesSendTarget ? handleSendFilePath : undefined}
                    sendTargetLabel={filesSendTarget ? getSessionNameFromKey(filesSendTarget) : null}
                  />
                </Suspense>
              </ErrorBoundary>
            )
          )}
          {activeTab === 'beads' && (
            <ErrorBoundary>
              <Suspense fallback={<ViewFallback />}>
                <BeadsView reveal={beadsRevealRequest} />
              </Suspense>
            </ErrorBoundary>
          )}
          {activeTab === 'agents' && (
            <ErrorBoundary>
              <Suspense fallback={<ViewFallback />}>
                <AgentsView onOpenInFiles={handleOpenProjectInFiles} />
              </Suspense>
            </ErrorBoundary>
          )}
          {activeTab === 'library' && (
            <ErrorBoundary>
              <Suspense fallback={<ViewFallback />}>
                <LibraryView />
              </Suspense>
            </ErrorBoundary>
          )}
          {activeTab === 'scheduled' && (
            <ErrorBoundary>
              <Suspense fallback={<ViewFallback />}>
                <ScheduledTasksView
                  activeWorkspaceId={lastActiveWorkspaceId}
                  sessionsDockState={sessionsDockState}
                  onSessionsDockStateChange={setSessionsDockState}
                  sessionsForcedPinned={sessionsForcedPinned}
                />
              </Suspense>
            </ErrorBoundary>
          )}
          {serverStatusTab && (
            <div style={{ display: activeTab === 'server' ? 'contents' : 'none' }}>
              <ErrorBoundary>
                <Suspense fallback={<ViewFallback />}>
                  <SystemStatusView active={activeTab === 'server'} />
                </Suspense>
              </ErrorBoundary>
            </div>
          )}
          {/* Even "static" views need a boundary: their lazy chunks can fail
              after a deploy, and an uncaught throw here unmounts the whole
              dashboard tree, terminal docks included. */}
          {activeTab === 'settings' && (
            <ErrorBoundary>
              <Suspense fallback={<ViewFallback />}>
                <SettingsView />
              </Suspense>
            </ErrorBoundary>
          )}
          {activeTab === 'help' && (
            <ErrorBoundary>
              <Suspense fallback={<ViewFallback />}>
                <HelpView />
              </Suspense>
            </ErrorBoundary>
          )}
          {/* Peek and the image glance float and the Send drawer overlays the
              right edge, all inside the workspace, so the status line stays
              whole beneath them. The table's column is each tab's own: a flex
              sibling of its content, never a layer here. */}
          <Peek />
          <ImageGlance />
          <SendDrawer />
          {/* The bottom-centre slot: the toast above, the key echo beneath. */}
          <Toast />
          <KeyEcho />
        </div>

        <StatusLine />

        {/* Outside the content column: the highlight is positioned against the
            viewport and must be able to reach every surface, tab bar included. */}
        <DevMode activeTab={activeTab} />

        <KeysPanel isOpen={keysPanelOpen} onClose={handleCloseKeys} />
      </div>
      </TableProvider>

      <DragOverlay className="drag-overlay-wrapper">
        {activeDrag ? <DraggedSessionOverlay drag={activeDrag} /> : null}
      </DragOverlay>
    </DndContext>
  )
}

function App() {
  return (
    <ThemeProvider>
      <StatusProvider>
        <SessionProvider>
          <TerminalPoolProvider>
            <DashboardContent />
          </TerminalPoolProvider>
        </SessionProvider>
      </StatusProvider>
    </ThemeProvider>
  )
}

export default App
