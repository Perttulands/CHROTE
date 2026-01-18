import { useState, useEffect } from 'react'
import { DndContext, DragEndEvent, DragStartEvent, DragOverlay, useSensor, useSensors, PointerSensor } from '@dnd-kit/core'
import { SessionProvider, useSession } from './context/SessionContext'
import TabBar from './components/TabBar'
import SessionPanel from './components/SessionPanel'
import TerminalArea from './components/TerminalArea'
import FilesView from './components/FilesView'
import SettingsView from './components/SettingsView'
import FloatingModal from './components/FloatingModal'
import HelpView from './components/HelpView'
import BeadsView from './components/BeadsView'
import ErrorBoundary from './components/ErrorBoundary'

type Tab = 'terminal1' | 'terminal2' | 'files' | 'beads' | 'settings' | 'help'

// Dragged item overlay component
function DraggedSessionOverlay({ name }: { name: string }) {
  const displayName = name.includes('-') ? name.split('-').slice(-1)[0] : name
  return (
    <div className="session-item dragging-overlay">
      <span className="session-agent-name">{displayName}</span>
    </div>
  )
}

function DashboardContent() {
  const [activeTab, setActiveTab] = useState<Tab>('terminal1')
  const [activeDragId, setActiveDragId] = useState<string | null>(null)
  const { addSessionToWindow, removeSessionFromWindow, setIsDragging, isDragging, settings } = useSession()

  // Apply theme to document
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', settings.theme)
  }, [settings.theme])

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
      setActiveDragId(event.active.data.current?.sessionName ?? null)
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
        const { sessionName, sourceWindowId, sourceWorkspaceId } = active.data.current
        removeSessionFromWindow(sourceWorkspaceId, sourceWindowId, sessionName)
      }
      return
    }

    // Dropped on a window
    if (over.data.current?.type === 'window') {
      const targetWindowId = over.data.current.windowId
      const targetWorkspaceId = over.data.current.workspaceId as 'terminal1' | 'terminal2'

      if (active.data.current?.type === 'session') {
        // Dragging from panel
        addSessionToWindow(targetWorkspaceId, targetWindowId, active.id as string)
      } else if (active.data.current?.type === 'tag') {
        // Dragging a tag between windows
        const { sessionName, sourceWindowId, sourceWorkspaceId } = active.data.current
        if (sourceWindowId !== targetWindowId || sourceWorkspaceId !== targetWorkspaceId) {
          addSessionToWindow(targetWorkspaceId, targetWindowId, sessionName)
        }
      }
    }
  }

  return (
    <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className={`dashboard ${isDragging ? 'is-dragging' : ''}`}>
        <TabBar activeTab={activeTab} onTabChange={setActiveTab} />

        <div className="dashboard-content">
          {(activeTab === 'terminal1' || activeTab === 'terminal2') && (
            <>
              <SessionPanel />
              <TerminalArea workspaceId={activeTab} />
            </>
          )}
          {activeTab === 'files' && <FilesView />}
          {activeTab === 'beads' && (
            <ErrorBoundary>
              <BeadsView />
            </ErrorBoundary>
          )}
          {activeTab === 'settings' && <SettingsView />}
          {activeTab === 'help' && <HelpView />}
        </div>

        <FloatingModal />
      </div>

      <DragOverlay>
        {activeDragId ? <DraggedSessionOverlay name={activeDragId} /> : null}
      </DragOverlay>
    </DndContext>
  )
}

function App() {
  return (
    <SessionProvider>
      <DashboardContent />
    </SessionProvider>
  )
}

export default App
