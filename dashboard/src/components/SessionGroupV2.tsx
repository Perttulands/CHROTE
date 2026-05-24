import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import type { TmuxSession } from '../types'
import { getGroupDisplayName } from '../types'
import SessionItem from './SessionItemV2'

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
        role="button"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            setExpanded(!expanded)
          }
        }}
        aria-expanded={expanded}
      >
        <span className="expand-icon">
          {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </span>
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
