import { useMemo, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { getGroupPriority } from '../types'
import SessionGroup from './SessionGroup'

function SessionPanel() {
  const { groupedSessions, loading, error, sidebarCollapsed, toggleSidebar, refreshSessions } = useSession()
  const [creating, setCreating] = useState(false)

  // Sort groups by priority
  const sortedGroups = useMemo(() => {
    return Object.entries(groupedSessions).sort(([a], [b]) => {
      const priorityA = getGroupPriority(a)
      const priorityB = getGroupPriority(b)
      if (priorityA !== priorityB) return priorityA - priorityB
      return a.localeCompare(b)
    })
  }, [groupedSessions])

  const createSession = async () => {
    setCreating(true)
    try {
      const response = await fetch('/api/tmux/sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({})
      })
      if (response.ok) {
        refreshSessions()
      }
    } catch (e) {
      console.error('Failed to create session:', e)
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className={`session-panel ${sidebarCollapsed ? 'collapsed' : ''}`}>
      <div className="session-panel-header">
        <button className="toggle-btn" onClick={toggleSidebar} title={sidebarCollapsed ? 'Expand' : 'Collapse'}>
          {sidebarCollapsed ? '»' : '«'}
        </button>
        {!sidebarCollapsed && (
          <>
            <span className="panel-title">Sessions</span>
            <button
              className="add-btn"
              onClick={createSession}
              disabled={creating}
              title="New tmux session"
            >
              +
            </button>
            <button className="refresh-btn" onClick={refreshSessions} title="Refresh sessions">
              ↻
            </button>
          </>
        )}
      </div>

      {!sidebarCollapsed && (
        <div className="session-panel-content">
          {loading && (
            <div className="panel-status">Loading...</div>
          )}

          {error && (
            <div className="panel-error">{error}</div>
          )}

          {!loading && !error && sortedGroups.length === 0 && (
            <div className="panel-status">No tmux sessions</div>
          )}

          {sortedGroups.map(([groupKey, sessions]) => (
            <SessionGroup key={groupKey} groupKey={groupKey} sessions={sessions} />
          ))}
        </div>
      )}
    </div>
  )
}

export default SessionPanel
