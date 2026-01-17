import { useSession } from '../context/SessionContext'
import TerminalWindow from './TerminalWindow'
import type { WorkspaceId } from '../types'

interface TerminalAreaProps {
  workspaceId: WorkspaceId
}

function TerminalArea({ workspaceId }: TerminalAreaProps) {
  const { workspaces, setWindowCount, isDragging } = useSession()
  const workspace = workspaces[workspaceId]
  const windows = workspace.windows
  const windowCount = workspace.windowCount

  // Get grid class based on window count
  const getGridClass = () => {
    switch (windowCount) {
      case 1: return 'grid-1'
      case 2: return 'grid-2'
      case 3: return 'grid-3'
      case 4: return 'grid-4'
      default: return 'grid-2'
    }
  }

  return (
    <div className="terminal-area">
      <div className="terminal-area-controls">
        <span className="layout-label">Layout:</span>
        {[1, 2, 3, 4].map(count => (
          <button
            key={count}
            className={`layout-btn ${windowCount === count ? 'active' : ''}`}
            onClick={() => setWindowCount(workspaceId, count)}
            title={`${count} window${count > 1 ? 's' : ''}`}
          >
            {count}
          </button>
        ))}
      </div>

      <div className={`terminal-grid ${getGridClass()}`}>
        {windows.slice(0, windowCount).map((window) => (
          <TerminalWindow
            key={window.id}
            workspaceId={workspaceId}
            window={window}
            isDragging={isDragging}
          />
        ))}
      </div>
    </div>
  )
}

export default TerminalArea
