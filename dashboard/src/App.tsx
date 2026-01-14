import { useState } from 'react'
import { DndContext, DragEndEvent, DragStartEvent, DragOverlay, useSensor, useSensors, PointerSensor } from '@dnd-kit/core'
import { SessionProvider, useSession } from './context/SessionContext'
import TabBar from './components/TabBar'
import SessionPanel from './components/SessionPanel'
import TerminalArea from './components/TerminalArea'
import FilesView from './components/FilesView'
import StatusView from './components/StatusView'
import FloatingModal from './components/FloatingModal'

type Tab = 'terminal' | 'files' | 'status'

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
  const [activeTab, setActiveTab] = useState<Tab>('terminal')
  const [activeDragId, setActiveDragId] = useState<string | null>(null)
  const { addSessionToWindow, removeSessionFromWindow, setIsDragging, isDragging } = useSession()

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 4, // Reduced from 8 for more responsive drag
      },
    })
  )

  const handleDragStart = (event: DragStartEvent) => {
    setActiveDragId(event.active.id as string)
    setIsDragging(true)
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    setActiveDragId(null)
    setIsDragging(false)

    if (!over) {
      // Dragged outside - if it's a tag, remove it from the window
      if (active.data.current?.type === 'tag') {
        const { sessionName, sourceWindowId } = active.data.current
        removeSessionFromWindow(sourceWindowId, sessionName)
      }
      return
    }

    // Dropped on a window
    if (over.data.current?.type === 'window') {
      const targetWindowId = over.data.current.windowId

      if (active.data.current?.type === 'session') {
        // Dragging from panel
        addSessionToWindow(targetWindowId, active.id as string)
      } else if (active.data.current?.type === 'tag') {
        // Dragging a tag between windows
        const { sessionName, sourceWindowId } = active.data.current
        if (sourceWindowId !== targetWindowId) {
          removeSessionFromWindow(sourceWindowId, sessionName)
          addSessionToWindow(targetWindowId, sessionName)
        }
      }
    }
  }

  return (
    <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className={`dashboard ${isDragging ? 'is-dragging' : ''}`}>
        <TabBar activeTab={activeTab} onTabChange={setActiveTab} />

        <div className="dashboard-content">
          {activeTab === 'terminal' && (
            <>
              <SessionPanel />
              <TerminalArea />
            </>
          )}
          {activeTab === 'files' && <FilesView />}
          {activeTab === 'status' && <StatusView />}
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
