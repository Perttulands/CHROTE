// Activity feed component for Oracle view

import type { ActivityEntry } from './types'

interface ActivityFeedProps {
  activity: ActivityEntry[]
}

function formatTime(timestamp: string): string {
  try {
    const date = new Date(timestamp)
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch {
    return '--:--:--'
  }
}

const EVENT_ICONS: Record<string, string> = {
  agent_new: '+',
  agent_status: '~',
  agent_removed: '-',
}

export default function ActivityFeed({ activity }: ActivityFeedProps) {
  if (activity.length === 0) {
    return (
      <div className="oracle-activity-feed">
        <div className="oracle-activity-empty">No activity yet. Events will appear here as agents change state.</div>
      </div>
    )
  }

  return (
    <div className="oracle-activity-feed">
      {activity.map(entry => (
        <div key={entry.id} className={`oracle-activity-entry oracle-activity-${entry.type}`}>
          <span className="oracle-activity-icon">{EVENT_ICONS[entry.type] || '?'}</span>
          <span className="oracle-activity-time">{formatTime(entry.timestamp)}</span>
          <span className="oracle-activity-msg">{entry.message}</span>
        </div>
      ))}
    </div>
  )
}
