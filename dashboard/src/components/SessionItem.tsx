import { useDraggable } from '@dnd-kit/core'
import type { TmuxSession } from '../types'
import { useSession } from '../context/SessionContext'
import { WINDOW_COLORS } from '../types'

interface SessionItemProps {
  session: TmuxSession
}

function SessionItem({ session }: SessionItemProps) {
  const { assignedSessions, handleSessionClick } = useSession()
  const assignment = assignedSessions.get(session.name)
  const isAssigned = !!assignment

  const badgeStyle = assignment
    ? {
        backgroundColor: WINDOW_COLORS[assignment.colorIndex % WINDOW_COLORS.length].border,
        color: '#000',
        borderColor: WINDOW_COLORS[assignment.colorIndex % WINDOW_COLORS.length].border
      }
    : undefined

  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: session.name,
    data: { type: 'session', session },
  })

  const style = transform
    ? {
        transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
        zIndex: isDragging ? 1000 : undefined,
      }
    : undefined

  return (
    <div
      ref={setNodeRef}
      className={`session-item ${isAssigned ? 'assigned' : ''} ${isDragging ? 'dragging' : ''}`}
      style={style}
      {...listeners}
      {...attributes}
      onClick={() => handleSessionClick(session.name)}
    >
      {assignment && (
        <span className="window-badge" style={badgeStyle}>
          {assignment.windowIndex}
        </span>
      )}
      <span className="session-agent-name">{session.agentName}</span>
      {session.attached && !isAssigned && <span className="attached-indicator" title="Attached elsewhere">●</span>}
    </div>
  )
}

export default SessionItem
