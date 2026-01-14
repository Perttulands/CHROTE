import { useDraggable } from '@dnd-kit/core'
import type { TmuxSession } from '../types'
import { useSession } from '../context/SessionContext'

interface SessionItemProps {
  session: TmuxSession
}

function SessionItem({ session }: SessionItemProps) {
  const { assignedSessions, handleSessionClick } = useSession()
  const isAssigned = assignedSessions.has(session.name)

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
      <span className="session-agent-name">{session.agentName}</span>
      {session.attached && <span className="attached-indicator" title="Attached">●</span>}
    </div>
  )
}

export default SessionItem
