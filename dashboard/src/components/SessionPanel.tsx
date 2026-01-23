import { useMemo, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { useToast } from '../context/ToastContext'
import { getGroupPriority } from '../types'
import SessionGroup from './SessionGroup'
import NukeConfirmModal from './NukeConfirmModal'

function SessionPanel() {
  const { groupedSessions, loading, error, sidebarCollapsed, toggleSidebar, refreshSessions, sessions, settings } = useSession()
  const { addToast } = useToast()
  const [creating, setCreating] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [showNukeModal, setShowNukeModal] = useState(false)
  const [nuking, setNuking] = useState(false)

  // Sort groups by priority and filter by search
  const sortedGroups = useMemo(() => {
    // Determine which groups/sessions to show
    const entries = Object.entries(groupedSessions)
    
    // Filter
    const filtered = searchTerm
      ? entries.map(([key, sessions]) => ([
          key,
          sessions.filter(s =>
            s.name.toLowerCase().includes(searchTerm.toLowerCase())
          )
        ] as [string, typeof sessions])).filter(([_, sessions]) => sessions.length > 0)
      : entries

    return filtered.sort(([a], [b]) => {
      const priorityA = getGroupPriority(a)
      const priorityB = getGroupPriority(b)
      if (priorityA !== priorityB) return priorityA - priorityB
      return a.localeCompare(b)
    })
  }, [groupedSessions, searchTerm])

  const createSession = async () => {
    setCreating(true)
    try {
      const prefix = settings.defaultSessionPrefix || 'tmux'
      // Escape regex special chars in prefix
      const escapedPrefix = prefix.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      const regex = new RegExp(`^${escapedPrefix}(\\d+)$`)

      // Find the next available number for this prefix
      const existingNumbers = sessions
        .map(s => s.name.match(regex))
        .filter(Boolean)
        .map(m => parseInt(m![1], 10))
        
      const nextNum = existingNumbers.length > 0
        ? Math.max(...existingNumbers) + 1
        : 1
      const sessionName = `${prefix}${nextNum}`
      
      const response = await fetch('/api/tmux/sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: sessionName })
      })
      if (response.ok) {
        addToast(`Session '${sessionName}' created`, 'success')
        refreshSessions()
      } else {
        addToast('Failed to create session', 'error')
      }
    } catch (e) {
      console.error('Failed to create session:', e)
      addToast('Failed to create session', 'error')
    } finally {
      setCreating(false)
    }
  }

  const nukeAllSessions = async () => {
    setNuking(true)
    try {
      const response = await fetch('/api/tmux/sessions/all', {
        method: 'DELETE',
        headers: {
          'X-Nuke-Confirm': 'DASHBOARD-NUKE-CONFIRMED'
        }
      })
      if (response.ok) {
        addToast('All sessions destroyed', 'warning')
        refreshSessions()
      } else {
        console.error('Failed to nuke sessions:', await response.text())
        addToast('Failed to destroy sessions', 'error')
      }
    } catch (e) {
      console.error('Failed to nuke sessions:', e)
      addToast('Failed to destroy sessions', 'error')
    } finally {
      setNuking(false)
      setShowNukeModal(false)
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
        <div className="session-search-container">
          <input
            type="text"
            className="session-search-input"
            placeholder="Filter sessions..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
      )}

      {!sidebarCollapsed && (
        <div className="session-panel-content">
          {loading && (
            <div className="panel-status">Loading...</div>
          )}

          {error && (
            <div className="panel-error">{error}</div>
          )}

          {!loading && !error && sortedGroups.length === 0 && (
            <div className="getting-started">
              <div className="getting-started-title">Getting Started</div>
              <div className="getting-started-text">
                Click <strong>+</strong> above to create your first tmux session,
                or use the terminal to run <code>tmux new -s mysession</code>
              </div>
              <div className="getting-started-hint">
                Sessions appear here and can be dragged to terminal windows
              </div>
            </div>
          )}

          {sortedGroups.map(([groupKey, groupSessions]) => (
            <SessionGroup key={groupKey} groupKey={groupKey} sessions={groupSessions} />
          ))}
        </div>
      )}

      {!sidebarCollapsed && sessions.length > 0 && (
        <div className="session-panel-footer">
          <button
            className="nuke-trigger-btn"
            onClick={() => setShowNukeModal(true)}
            disabled={nuking}
            title="Destroy all tmux sessions"
          >
            ☢ {nuking ? 'Nuking...' : 'Nuke All'}
          </button>
        </div>
      )}

      {showNukeModal && (
        <NukeConfirmModal
          sessionCount={sessions.length}
          sessionNames={sessions.map(s => s.name)}
          onConfirm={nukeAllSessions}
          onCancel={() => setShowNukeModal(false)}
        />
      )}
    </div>
  )
}

export default SessionPanel
