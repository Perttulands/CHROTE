import { Suspense, lazy, useCallback, useEffect, useMemo, useState } from 'react'
import { DndContext, DragEndEvent, DragStartEvent, DragOverlay, useSensor, useSensors, PointerSensor } from '@dnd-kit/core'
import { SessionProvider, useSession } from './context/SessionContext'
import TabBar, { Tab } from './components/TabBar'
import TerminalWorkspaceDock from './components/TerminalWorkspaceDock'
import FloatingModal from './components/FloatingModal'
import SendToSessionModal from './components/SendToSessionModal'
import ErrorBoundary from './components/ErrorBoundary'
import Skeleton from './components/LoadingSkeleton'
import { ToastContainer } from './components/ToastNotification'
import KeyboardShortcutsOverlay from './components/KeyboardShortcutsOverlay'
import LayoutPresetsPanel from './components/LayoutPresetsPanel'
import { IframePoolProvider } from './components/IframePool'
import {
  readSessionsDockState,
  writeSessionsDockState,
  type SessionsDockState,
} from './components/workspaceFilesState'
import { useKeyboardShortcuts } from './hooks/useKeyboardShortcuts'
import { installFeatureFlagHelpers, isFeatureEnabled } from './featureFlags'
import { getSessionNameFromKey, getTerminalUserColor, getTerminalUserInitial, isTerminalWorkspaceId, sortTerminalWorkspaceIds } from './types'
import type { UserSettings, WorkspaceId } from './types'

// Non-terminal views load as route-level chunks on first visit. The terminal
// docks and everything they depend on stay eager: they are the startup surface
// and their iframe pool must never be interrupted by a chunk load. Each lazy
// view gets its OWN Suspense boundary inside its mount branch — one shared
// boundary around .dashboard-content would swap the whole tree (terminal docks
// included) for a fallback while a chunk loads, killing live terminals.
const FilesView = lazy(() => import('./components/FilesView'))
const SettingsView = lazy(() => import('./components/SettingsView'))
const HelpView = lazy(() => import('./components/HelpView'))
const BeadsView = lazy(() => import('./components/BeadsView'))
const FormationsCockpit = lazy(() => import('./components/FormationsCockpit'))
const AgentsView = lazy(() => import('./components/AgentsView'))
const ServicesView = lazy(() => import('./components/ServicesView'))
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

// Dragged item overlay component
function DraggedSessionOverlay({ drag, settings }: { drag: ActiveDrag; settings: UserSettings }) {
  const { name, type, unixUser } = drag
  const displayName = getSessionNameFromKey(name)
  const badgeClassName = type === 'tag' ? 'session-user-badge' : 'unix-user-badge'
  return (
    <div className={`${type === 'tag' ? 'session-tag' : 'session-item'} dragging-overlay`}>
      {unixUser && (
        <span
          className={badgeClassName}
          style={{ backgroundColor: getTerminalUserColor(settings, unixUser) }}
          title={`Unix user: ${unixUser}`}
          aria-label={`Unix user ${unixUser}`}
        >
          {getTerminalUserInitial(unixUser)}
        </span>
      )}
      <span className={type === 'tag' ? 'tag-name' : 'session-agent-name'}>{displayName}</span>
    </div>
  )
}

function DashboardContent() {
  const [activeTab, setActiveTab] = useState<Tab>('terminal1')
  const [lastActiveWorkspaceId, setLastActiveWorkspaceId] = useState<WorkspaceId>('terminal1')
  const [sessionsDockState, setSessionsDockState] = useState<SessionsDockState>(readSessionsDockState)
  const [openFilesWorkspaceIds, setOpenFilesWorkspaceIds] = useState<Set<WorkspaceId>>(() => new Set())
  const [activeDrag, setActiveDrag] = useState<ActiveDrag | null>(null)
  const [showHelp, setShowHelp] = useState(false)
  const [showPresets, setShowPresets] = useState(false)
  const [settingsSessionBankFocusNonce, setSettingsSessionBankFocusNonce] = useState(0)
  const [filesNavigateRequest, setFilesNavigateRequest] = useState<{ path: string; nonce: number } | null>(null)
  const [formationsVisited, setFormationsVisited] = useState(false)
  const {
    addSessionToWindow,
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
      if (terminalWindow?.activeSession && terminalWindow.activeSession !== 'INIT-PENDING') return terminalWindow.activeSession
    }
    return null
  }, [focusedWindowKey, workspaces])
  const handleSendFilePath = useCallback((path: string) => {
    if (filesSendTarget) openSendToSession(filesSendTarget, path)
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
  // a shrunken tab count — so panel state and pooled iframe claims survive.
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

  const handleShowHelp = useCallback(() => setShowHelp(true), [])
  const handleCloseHelp = useCallback(() => setShowHelp(false), [])
  const handleShowPresets = useCallback(() => setShowPresets(true), [])
  const handleClosePresets = useCallback(() => setShowPresets(false), [])
  const handleTabChange = useCallback((tab: Tab) => {
    setActiveTab(tab)
    if (isTerminalWorkspaceId(tab, mountedWorkspaceIds)) setLastActiveWorkspaceId(tab)
  }, [mountedWorkspaceIds])
  const handleOpenProjectInFiles = useCallback((path: string) => {
    setFilesNavigateRequest({ path, nonce: Date.now() })
    setActiveTab('files')
  }, [])
  const handleOpenSessionBankSettings = useCallback(() => {
    setSettingsSessionBankFocusNonce(Date.now())
    setActiveTab('settings')
  }, [])

  useEffect(() => {
    if (windowRevealRequest) {
      setActiveTab(windowRevealRequest.workspaceId)
      setLastActiveWorkspaceId(windowRevealRequest.workspaceId)
    }
  }, [windowRevealRequest])

  // Global keyboard shortcuts
  useKeyboardShortcuts({
    onShowHelp: handleShowHelp,
    isHelpOpen: showHelp,
  })

  // Apply theme to document
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', settings.theme)
  }, [settings.theme])

  useEffect(() => {
    installFeatureFlagHelpers()
  }, [])

  // Once visited, Formations stays mounted (hidden) so canvas/viewport state
  // survives tab switches, mirroring the terminal tabs' keep-alive pattern.
  useEffect(() => {
    if (activeTab === 'formations') setFormationsVisited(true)
  }, [activeTab])

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
    const targetData = readWindowDropData(over.data.current)
    if (!dragData || !targetData) return

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
      <div className={`dashboard ${activeDrag ? 'is-dragging' : ''}`}>
        <TabBar
          activeTab={activeTab}
          onTabChange={handleTabChange}
          onShowHelp={handleShowHelp}
          onShowPresets={handleShowPresets}
        />

        <div className="dashboard-content">
          {/* Terminal workspaces stay mounted so panel state and pooled iframe connections survive tab switches. */}
          {mountedWorkspaceIds.map(workspaceId => (
            <TerminalWorkspaceDock
              key={workspaceId}
              workspaceId={workspaceId}
              active={activeTab === workspaceId}
              sessionsDockState={sessionsDockState}
              onSessionsDockStateChange={setSessionsDockState}
              sessionsForcedPinned={sessionsForcedPinned}
              onFilesOpenChange={handleFilesOpenChange}
              onOpenSessionBankSettings={handleOpenSessionBankSettings}
              onOpenInFiles={handleOpenProjectInFiles}
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
                <BeadsView onOpenProjectInFiles={handleOpenProjectInFiles} />
              </Suspense>
            </ErrorBoundary>
          )}
          {(formationsVisited || activeTab === 'formations') && (
            <div
              className="formations-host"
              data-testid="formations-host"
              style={{
                display: activeTab === 'formations' ? 'flex' : 'none',
                position: 'relative',
                flex: 1,
                minWidth: 0,
              }}
            >
              <ErrorBoundary>
                <Suspense fallback={<ViewFallback />}>
                  <FormationsCockpit active={activeTab === 'formations'} />
                </Suspense>
              </ErrorBoundary>
            </div>
          )}
          {activeTab === 'agents' && (
            <ErrorBoundary>
              <Suspense fallback={<ViewFallback />}>
                <AgentsView />
              </Suspense>
            </ErrorBoundary>
          )}
          {activeTab === 'services' && (
            <ErrorBoundary>
              <Suspense fallback={<ViewFallback />}>
                <ServicesView />
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
                <SettingsView sessionBankFocusNonce={settingsSessionBankFocusNonce} />
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
        </div>

        <FloatingModal />
        <SendToSessionModal />

        {/* Overlays */}
        <KeyboardShortcutsOverlay isOpen={showHelp} onClose={handleCloseHelp} />
        <LayoutPresetsPanel isOpen={showPresets} onClose={handleClosePresets} />
      </div>

      {/* Toast notifications */}
      <ToastContainer />

      <DragOverlay className="drag-overlay-wrapper">
        {activeDrag ? <DraggedSessionOverlay drag={activeDrag} settings={settings} /> : null}
      </DragOverlay>
    </DndContext>
  )
}

function App() {
  return (
    <SessionProvider>
      <IframePoolProvider>
        <DashboardContent />
      </IframePoolProvider>
    </SessionProvider>
  )
}

export default App
