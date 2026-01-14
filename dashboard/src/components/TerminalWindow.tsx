import { useState } from 'react'
import { useDroppable, useDraggable } from '@dnd-kit/core'
import { useSession } from '../context/SessionContext'
import { WINDOW_COLORS } from '../types'
import type { TerminalWindow as TerminalWindowType } from '../types'

interface DropOverlayProps {
  windowId: string
  isVisible: boolean
}

// Full-window drop overlay that appears during drag
function DropOverlay({ windowId, isVisible }: DropOverlayProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: `drop-${windowId}`,
    data: { type: 'window', windowId },
  })

  if (!isVisible) return null

  return (
    <div
      ref={setNodeRef}
      className={`terminal-drop-overlay ${isOver ? 'is-over' : ''}`}
    >
      <span className="drop-hint">{isOver ? 'Release to add' : 'Drop here'}</span>
    </div>
  )
}

interface SessionTagProps {
  sessionName: string
  isActive: boolean
  windowId: string
  onRemove: () => void
  onClick: () => void
}

function SessionTag({ sessionName, isActive, windowId, onRemove, onClick }: SessionTagProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `tag-${windowId}-${sessionName}`,
    data: { type: 'tag', sessionName, sourceWindowId: windowId },
  })

  const style = transform
    ? {
        transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
        zIndex: isDragging ? 1000 : undefined,
      }
    : undefined

  // Extract just the agent name for display
  const displayName = sessionName.includes('-')
    ? sessionName.split('-').slice(-1)[0]
    : sessionName

  return (
    <div
      ref={setNodeRef}
      className={`session-tag ${isActive ? 'active' : ''} ${isDragging ? 'dragging' : ''}`}
      style={style}
      {...listeners}
      {...attributes}
    >
      <span className="tag-name" onClick={onClick}>{displayName}</span>
      <button className="tag-remove" onClick={(e) => { e.stopPropagation(); onRemove(); }}>×</button>
    </div>
  )
}

interface TerminalWindowProps {
  window: TerminalWindowType
  isDragging?: boolean
}

function TerminalWindow({ window: windowConfig, isDragging = false }: TerminalWindowProps) {
  const [loaded, setLoaded] = useState(false)

  const {
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
  } = useSession()

  const colorTheme = WINDOW_COLORS[windowConfig.colorIndex % WINDOW_COLORS.length]

  const handleRemoveSession = (sessionName: string) => {
    removeSessionFromWindow(windowConfig.id, sessionName)
  }

  const handleTagClick = (sessionName: string) => {
    setActiveSession(windowConfig.id, sessionName)
  }

  const hasSessions = windowConfig.boundSessions.length > 0
  const activeSession = windowConfig.activeSession

  // ttyd serves its own terminal UI - embed via iframe
  // If a session is active, pass it as arg to attach to that tmux session
  const terminalUrl = activeSession
    ? `/terminal/?arg=${encodeURIComponent(activeSession)}`
    : `/terminal/`

  return (
    <div
      className="terminal-window"
      style={{
        '--window-accent': colorTheme.accent,
        '--window-bg': colorTheme.bg,
        '--window-border': colorTheme.border,
      } as React.CSSProperties}
    >
      <div className="terminal-window-header">
        <div className="session-tags">
          {windowConfig.boundSessions.map(sessionName => (
            <SessionTag
              key={sessionName}
              sessionName={sessionName}
              isActive={sessionName === activeSession}
              windowId={windowConfig.id}
              onRemove={() => handleRemoveSession(sessionName)}
              onClick={() => handleTagClick(sessionName)}
            />
          ))}
          {!hasSessions && <span className="no-sessions">Shell ready</span>}
        </div>

        <div className="window-controls">
          {hasSessions && windowConfig.boundSessions.length > 1 && (
            <>
              <button
                className="cycle-btn"
                onClick={() => cycleSession(windowConfig.id, 'prev')}
                title="Previous session"
              >
                ←
              </button>
              <button
                className="cycle-btn"
                onClick={() => cycleSession(windowConfig.id, 'next')}
                title="Next session"
              >
                →
              </button>
            </>
          )}
          <span className={`status-dot ${loaded ? '' : 'disconnected'}`} />
        </div>
      </div>

      <div className="terminal-window-body">
        {/* Key forces iframe reload when session changes */}
        <iframe
          key={activeSession || 'shell'}
          src={terminalUrl}
          onLoad={() => setLoaded(true)}
          style={{
            width: '100%',
            height: '100%',
            border: 'none',
            backgroundColor: colorTheme.bg,
          }}
          title={`Terminal ${windowConfig.id}${activeSession ? ` - ${activeSession}` : ''}`}
        />
        <DropOverlay windowId={windowConfig.id} isVisible={isDragging} />
      </div>
    </div>
  )
}

export default TerminalWindow
