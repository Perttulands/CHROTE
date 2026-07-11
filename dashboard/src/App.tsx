import { useState, useEffect, useCallback } from 'react'
import { DndContext, DragEndEvent, DragStartEvent, DragOverlay, useSensor, useSensors, PointerSensor } from '@dnd-kit/core'
import { SessionProvider, useSession } from './context/SessionContext'
import TabBar, { Tab } from './components/TabBar'
import SessionPanel from './components/SessionPanel'
import TerminalArea from './components/TerminalArea'
import FilesView from './components/FilesView'
import SettingsView from './components/SettingsView'
import FloatingModal from './components/FloatingModal'
import SendToSessionModal from './components/SendToSessionModal'
import HelpView from './components/HelpView'
import BeadsView from './components/BeadsView'
import FormationsCockpit from './components/FormationsCockpit'
import AgentsView from './components/AgentsView'
import ServicesView from './components/ServicesView'
import SystemStatusView from './components/SystemStatusView'
import ScheduledTasksView from './components/ScheduledTasksView'
import ErrorBoundary from './components/ErrorBoundary'
import { ToastContainer } from './components/ToastNotification'
import KeyboardShortcutsOverlay from './components/KeyboardShortcutsOverlay'
import LayoutPresetsPanel from './components/LayoutPresetsPanel'
import { IframePoolProvider } from './components/IframePool'
import { useKeyboardShortcuts } from './hooks/useKeyboardShortcuts'
import { installFeatureFlagHelpers, isFeatureEnabled } from './featureFlags'
import { TERMINAL_WORKSPACE_IDS, getSessionNameFromKey } from './types'
import type { WorkspaceId } from './types'

// Dragged item overlay component
function DraggedSessionOverlay({ name }: { name: string }) {
  const displayName = getSessionNameFromKey(name)
  return (
    <div className="session-item dragging-overlay">
      <span className="session-agent-name">{displayName}</span>
    </div>
  )
}

function DashboardContent() {
  const [activeTab, setActiveTab] = useState<Tab>('terminal1')
  const [activeDragId, setActiveDragId] = useState<string | null>(null)
  const [showHelp, setShowHelp] = useState(false)
  const [showPresets, setShowPresets] = useState(false)
  const [settingsSessionBankFocusNonce, setSettingsSessionBankFocusNonce] = useState(0)
  const [filesNavigateRequest, setFilesNavigateRequest] = useState<{ path: string; nonce: number } | null>(null)
  const [formationsVisited, setFormationsVisited] = useState(false)
  const { addSessionToWindow, removeSessionFromWindow, setIsDragging, isDragging, settings } = useSession()
  const persistFilesTabState = isFeatureEnabled('filesPersistTabState')
  const serverStatusTab = isFeatureEnabled('serverStatusTab')

  const handleShowHelp = useCallback(() => setShowHelp(true), [])
  const handleCloseHelp = useCallback(() => setShowHelp(false), [])
  const handleShowPresets = useCallback(() => setShowPresets(true), [])
  const handleClosePresets = useCallback(() => setShowPresets(false), [])
  const handleTabChange = useCallback((tab: Tab) => {
    setActiveTab(tab)
  }, [])
  const handleOpenProjectInFiles = useCallback((path: string) => {
    setFilesNavigateRequest({ path, nonce: Date.now() })
    setActiveTab('files')
  }, [])
  const handleOpenSessionBankSettings = useCallback(() => {
    setSettingsSessionBankFocusNonce(Date.now())
    setActiveTab('settings')
  }, [])

  // Global keyboard shortcuts
  useKeyboardShortcuts({
    activeTab,
    onTabChange: handleTabChange,
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

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 4, // Reduced from 8 for more responsive drag
      },
    })
  )

  const handleDragStart = (event: DragStartEvent) => {
    const type = event.active.data.current?.type
    if (type === 'tag') {
      setActiveDragId(event.active.data.current?.sessionKey ?? event.active.data.current?.sessionName ?? null)
    } else {
      setActiveDragId(event.active.id as string)
    }
    setIsDragging(true)
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    setActiveDragId(null)
    setIsDragging(false)

    if (!over) {
      // Dragged outside - if it's a tag, remove it from the window
      if (active.data.current?.type === 'tag') {
        const { sessionName, sessionKey, sourceWindowId, sourceWorkspaceId } = active.data.current
        removeSessionFromWindow(sourceWorkspaceId, sourceWindowId, sessionKey ?? sessionName)
      }
      return
    }

    // Dropped on a window
    if (over.data.current?.type === 'window') {
      const targetWindowId = over.data.current.windowId
      const targetWorkspaceId = over.data.current.workspaceId as WorkspaceId

      if (active.data.current?.type === 'session') {
        // Dragging from panel
        const { sessionName, unixUser } = active.data.current
        addSessionToWindow(targetWorkspaceId, targetWindowId, sessionName ?? active.id as string, unixUser)
      } else if (active.data.current?.type === 'tag') {
        // Dragging a tag between windows
        const { sessionName, sessionKey, unixUser, sourceWindowId, sourceWorkspaceId } = active.data.current
        if (sourceWindowId !== targetWindowId || sourceWorkspaceId !== targetWorkspaceId) {
          addSessionToWindow(targetWorkspaceId, targetWindowId, sessionName ?? sessionKey, unixUser)
        }
      }
    }
  }

  return (
    <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className={`dashboard ${isDragging ? 'is-dragging' : ''}`}>
        <TabBar
          activeTab={activeTab}
          onTabChange={handleTabChange}
          onShowHelp={handleShowHelp}
          onShowPresets={handleShowPresets}
        />

        <div className="dashboard-content">
          {/* Terminal areas are always rendered (hidden via CSS) to preserve iframe connections */}
          <div style={{ display: TERMINAL_WORKSPACE_IDS.includes(activeTab as WorkspaceId) ? 'contents' : 'none' }}>
            <SessionPanel onOpenSessionBankSettings={handleOpenSessionBankSettings} />
          </div>
          {TERMINAL_WORKSPACE_IDS.map(workspaceId => (
            <div key={workspaceId} style={{ display: activeTab === workspaceId ? 'contents' : 'none' }}>
              <TerminalArea workspaceId={workspaceId} />
            </div>
          ))}
          {persistFilesTabState ? (
            <div style={{ display: activeTab === 'files' ? 'contents' : 'none' }}>
              <FilesView navigateRequest={filesNavigateRequest} />
            </div>
          ) : (
            activeTab === 'files' && <FilesView navigateRequest={filesNavigateRequest} />
          )}
          {activeTab === 'beads' && (
            <ErrorBoundary>
              <BeadsView onOpenProjectInFiles={handleOpenProjectInFiles} />
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
                <FormationsCockpit active={activeTab === 'formations'} />
              </ErrorBoundary>
            </div>
          )}
          {activeTab === 'agents' && (
            <ErrorBoundary>
              <AgentsView />
            </ErrorBoundary>
          )}
          {activeTab === 'services' && (
            <ErrorBoundary>
              <ServicesView />
            </ErrorBoundary>
          )}
          {activeTab === 'scheduled' && (
            <ErrorBoundary>
              <ScheduledTasksView />
            </ErrorBoundary>
          )}
          {serverStatusTab && (
            <div style={{ display: activeTab === 'server' ? 'contents' : 'none' }}>
              <ErrorBoundary>
                <SystemStatusView active={activeTab === 'server'} />
              </ErrorBoundary>
            </div>
          )}
          {activeTab === 'settings' && <SettingsView sessionBankFocusNonce={settingsSessionBankFocusNonce} />}
          {activeTab === 'help' && <HelpView />}
        </div>

        <FloatingModal />
        <SendToSessionModal />

        {/* Overlays */}
        <KeyboardShortcutsOverlay isOpen={showHelp} onClose={handleCloseHelp} />
        <LayoutPresetsPanel isOpen={showPresets} onClose={handleClosePresets} />
      </div>

      {/* Toast notifications */}
      <ToastContainer />

      <DragOverlay>
        {activeDragId ? <DraggedSessionOverlay name={activeDragId} /> : null}
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
