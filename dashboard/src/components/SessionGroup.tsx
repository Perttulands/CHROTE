import type { TmuxSession } from '../types'
import { getGroupDisplayName, getSessionKey } from '../types'
import SessionItem, { type SelectGesture } from './SessionItem'

interface SessionGroupProps {
  groupKey: string
  sessions: TmuxSession[]
  expanded: boolean
  onExpandedChange: (expanded: boolean) => void
  selectedKeys?: ReadonlySet<string>
  onSelectGesture?: (sessionKey: string, gesture: SelectGesture) => void
  onSelectionMenu?: (at: { x: number; y: number }) => void
}

function SessionGroup({ groupKey, sessions, expanded, onExpandedChange, selectedKeys, onSelectGesture, onSelectionMenu }: SessionGroupProps) {
  const displayName = getGroupDisplayName(groupKey)

  return (
    <div className="session-group">
      <div
        className="session-group-header"
        onClick={() => onExpandedChange(!expanded)}
      >
        <span className="expand-icon">{expanded ? '▼' : '▶'}</span>
        <span className="group-name">{displayName}</span>
        <span className="session-count">{sessions.length}</span>
      </div>

      {expanded && (
        <div className="session-group-items">
          {sessions.map(session => {
            const sessionKey = getSessionKey(session.name, session.unixUser)
            return (
              <SessionItem
                key={sessionKey}
                session={session}
                selected={selectedKeys?.has(sessionKey) ?? false}
                onSelectGesture={onSelectGesture}
                onSelectionMenu={onSelectionMenu}
              />
            )
          })}
        </div>
      )}
    </div>
  )
}

export default SessionGroup
