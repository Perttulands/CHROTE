import { useSession } from '../context/SessionContext'
import TerminalWindow from './TerminalWindow'

function TerminalArea() {
  const { windows, windowCount, setWindowCount, isDragging, focusedWindowIndex, setFocusedWindowIndex } = useSession()

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
            onClick={() => setWindowCount(count)}
            title={`${count} window${count > 1 ? 's' : ''}`}
          >
            {count}
          </button>
        ))}
      </div>

      <div className={`terminal-grid ${getGridClass()}`}>
        {windows.slice(0, windowCount).map((window, index) => (
          <TerminalWindow
            key={window.id}
            window={window}
            isDragging={isDragging}
            isFocused={index === focusedWindowIndex}
            onFocus={() => setFocusedWindowIndex(index)}
          />
        ))}
      </div>
    </div>
  )
}

export default TerminalArea
