import { useState } from 'react'
import type { TmuxSession } from '../types'
import { getGroupDisplayName } from '../types'
import SessionItem from './SessionItem'

interface SessionGroupProps {
  groupKey: string
  sessions: TmuxSession[]
}

function SessionGroup({ groupKey, sessions }: SessionGroupProps) {
  const [expanded, setExpanded] = useState(true)
  const displayName = getGroupDisplayName(groupKey)

  return (
    <div className="session-group">
      <div
        className="session-group-header"
        onClick={() => setExpanded(!expanded)}
      >
        <span className="expand-icon">{expanded ? '▼' : '▶'}</span>
        <span className="group-name">{displayName}</span>
        <span className="session-count">{sessions.length}</span>
      </div>

      {expanded && (
        <div className="session-group-items">
          {sessions.map(session => (
            <SessionItem key={session.name} session={session} />
          ))}
        </div>
      )}
    </div>
  )
}

export default SessionGroup
