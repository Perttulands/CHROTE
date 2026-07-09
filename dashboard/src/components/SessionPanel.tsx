import { useEffect, useMemo, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { useToast } from '../context/ToastContext'
import { getDefaultLaunchUser, getGroupPriority, getTerminalUserInitial } from '../types'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import { copyTextToClipboard } from '../utils/clipboard'
import SessionGroup from './SessionGroup'
import NukeConfirmModal from './NukeConfirmModal'

function SessionPanel() {
  const { groupedSessions, loading, error, sidebarCollapsed, toggleSidebar, refreshSessions, createSession: createSessionAction, sessions, sessionBank, terminalUsers, settings } = useSession()
  const { addToast } = useToast()
  const [creating, setCreating] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [showNukeModal, setShowNukeModal] = useState(false)
  const [nuking, setNuking] = useState(false)
  const [newSessionMenu, setNewSessionMenu] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const [namedSessionPopup, setNamedSessionPopup] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const [namedSessionName, setNamedSessionName] = useState('')
  const [namedSessionUser, setNamedSessionUser] = useState('')
  const [sessionBankCollapsed, setSessionBankCollapsed] = useState(false)
  const newSessionMenuPosition = useViewportMenuPosition<HTMLDivElement>(
    newSessionMenu.show ? { x: newSessionMenu.x, y: newSessionMenu.y } : null,
    { estimatedSize: { width: 190, height: 130 } },
  )
  const namedSessionPopupPosition = useViewportMenuPosition<HTMLDivElement>(
    namedSessionPopup.show ? { x: namedSessionPopup.x, y: namedSessionPopup.y } : null,
    { estimatedSize: { width: 240, height: 180 } },
  )

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

  const bankedSessions = useMemo(() => {
    const needle = searchTerm.trim().toLowerCase()
    return sessionBank
      .filter(session => !session.live)
      .filter(session => {
        if (!needle) return true
        return [
          session.name,
          session.unixUser ?? '',
          session.resumeCommand ?? '',
          session.agentKind ?? '',
          session.agentSessionId ?? '',
          session.cwd ?? '',
        ].some(value => value.toLowerCase().includes(needle))
      })
  }, [sessionBank, searchTerm])

  useEffect(() => {
    if (!newSessionMenu.show) return
    const close = (event: MouseEvent) => {
      if (newSessionMenuPosition.ref.current?.contains(event.target as Node)) return
      setNewSessionMenu({ show: false, x: 0, y: 0 })
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [newSessionMenu.show])

  useEffect(() => {
    if (!namedSessionPopup.show) return
    const close = (event: MouseEvent) => {
      if (namedSessionPopupPosition.ref.current?.contains(event.target as Node)) return
      setNamedSessionPopup({ show: false, x: 0, y: 0 })
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [namedSessionPopup.show])

  const createSessionForUser = async (unixUser?: string, explicitName?: string) => {
    setCreating(true)
    try {
      const created = await createSessionAction({
        workspaceId: 'terminal1',
        ...(unixUser !== undefined ? { unixUser } : {}),
        ...(explicitName !== undefined ? { name: explicitName } : {}),
      })
      if (created) {
        setNamedSessionName('')
        setNamedSessionPopup({ show: false, x: 0, y: 0 })
      }
    } finally {
      setCreating(false)
      setNewSessionMenu({ show: false, x: 0, y: 0 })
    }
  }

  const createSession = async () => {
    await createSessionForUser()
  }

  const openNamedSessionField = () => {
    const unixUser = getDefaultLaunchUser('terminal1', terminalUsers)
    setNamedSessionUser(unixUser)
    setNamedSessionPopup({ show: true, x: newSessionMenu.x, y: newSessionMenu.y })
    setNewSessionMenu({ show: false, x: 0, y: 0 })
  }

  const submitNamedSession = async () => {
    const unixUser = namedSessionUser || getDefaultLaunchUser('terminal1', terminalUsers)
    if (!namedSessionName.trim()) return
    await createSessionForUser(unixUser, namedSessionName)
  }

  const copyResumeCommand = async (resumeCommand: string) => {
    const copied = await copyTextToClipboard(resumeCommand)
    addToast(copied ? 'Resume command copied' : 'Failed to copy resume command', copied ? 'success' : 'error')
  }

  const recreateBankedSession = async (name: string, unixUser?: string) => {
    await createSessionForUser(unixUser, name)
  }

  const isRecoverableAgent = (session: typeof sessionBank[number]) => (
    session.recoveryKind === 'agent' && Boolean(session.agentKind && session.agentSessionId && session.resumeCommand?.trim())
  )

  const resumeBankedAgent = async (session: typeof sessionBank[number]) => {
    try {
      const query = new URLSearchParams()
      if (session.unixUser) query.set('unixUser', session.unixUser)
      const response = await fetch(`/api/tmux/session-bank/${encodeURIComponent(session.name)}/recover${query.toString() ? `?${query.toString()}` : ''}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mouseScroll: settings.mouseScroll }),
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        console.error('Failed to resume banked agent:', await response.text())
        addToast('Failed to resume agent', 'error')
        return
      }
      addToast(`Resumed agent ${session.name}`, 'success')
      await refreshSessions()
    } catch (e) {
      console.error('Failed to resume banked agent:', e)
      addToast('Failed to resume agent', 'error')
    }
  }

  const removeBankedSession = async (name: string, unixUser?: string) => {
    try {
      const query = new URLSearchParams()
      if (unixUser) query.set('unixUser', unixUser)
      const response = await fetch(`/api/tmux/session-bank/${encodeURIComponent(name)}${query.toString() ? `?${query.toString()}` : ''}`, {
        method: 'DELETE',
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        console.error('Failed to remove banked session:', await response.text())
        addToast('Failed to remove session bank entry', 'error')
        return
      }
      addToast(`Removed ${name} from session bank`, 'success')
      await refreshSessions()
    } catch (e) {
      console.error('Failed to remove banked session:', e)
      addToast('Failed to remove session bank entry', 'error')
    }
  }

  const nukeAllSessions = async () => {
    setNuking(true)
    try {
      const response = await fetch('/api/tmux/sessions/all', {
        method: 'DELETE',
        headers: {
          'X-Nuke-Confirm': 'DASHBOARD-NUKE-CONFIRMED'
        },
        signal: AbortSignal.timeout(10000),
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
              onContextMenu={(event) => {
                event.preventDefault()
                setNewSessionMenu({ show: true, x: event.clientX, y: event.clientY })
              }}
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

      {newSessionMenu.show && (
        <div
          ref={newSessionMenuPosition.ref}
          className="session-context-menu"
          style={newSessionMenuPosition.style}
        >
          {terminalUsers.map(user => (
            <button
              key={user}
              className="session-context-item"
              onClick={() => createSessionForUser(user)}
              disabled={creating}
            >
              <span className="session-context-icon">{getTerminalUserInitial(user)}</span>
              New as {getTerminalUserInitial(user)} {user}
            </button>
          ))}
          <div className="session-context-divider" />
          <button className="session-context-item" onClick={openNamedSessionField}>
            <span className="session-context-icon">✎</span>
            New named session
          </button>
        </div>
      )}

      {!sidebarCollapsed && namedSessionPopup.show && (
        <div
          ref={namedSessionPopupPosition.ref}
          role="dialog"
          aria-label="Create named tmux session"
          className="session-context-menu session-named-popup"
          style={namedSessionPopupPosition.style}
        >
          <div className="session-named-popup-title">New named session</div>
          <input
            aria-label="New session name"
            type="text"
            className="session-search-input"
            placeholder="Session name..."
            value={namedSessionName}
            onChange={(e) => setNamedSessionName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void submitNamedSession()
              if (e.key === 'Escape') setNamedSessionPopup({ show: false, x: 0, y: 0 })
            }}
            autoFocus
          />
          {terminalUsers.length > 1 && (
            <select
              aria-label="New named session user"
              className="session-user-select"
              value={namedSessionUser || getDefaultLaunchUser('terminal1', terminalUsers)}
              onChange={(e) => setNamedSessionUser(e.target.value)}
            >
              {terminalUsers.map(user => <option key={user} value={user}>{user}</option>)}
            </select>
          )}
          <div className="session-named-popup-actions">
            <button className="session-context-item session-inline-action" onClick={submitNamedSession} disabled={!namedSessionName.trim() || creating}>
              Create named session
            </button>
            <button className="session-context-item session-inline-action" onClick={() => setNamedSessionPopup({ show: false, x: 0, y: 0 })}>
              Cancel
            </button>
          </div>
        </div>
      )}

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

          {!loading && !error && bankedSessions.length > 0 && (
            <section className={`session-bank ${sessionBankCollapsed ? 'session-bank-collapsed' : ''}`} aria-label="Session bank">
              <div className="session-bank-header">
                <h2>Session bank</h2>
                <span className="session-bank-count" aria-hidden="true">{bankedSessions.length}</span>
                <button
                  type="button"
                  className="session-bank-toggle"
                  aria-label={sessionBankCollapsed ? 'Expand session bank' : 'Collapse session bank'}
                  aria-expanded={!sessionBankCollapsed}
                  aria-controls="session-bank-list"
                  onClick={() => setSessionBankCollapsed(collapsed => !collapsed)}
                >
                  {sessionBankCollapsed ? '▸' : '▾'}
                </button>
              </div>
              {!sessionBankCollapsed && (
                <div id="session-bank-list" className="session-bank-list">
                  <p>Offline sessions seen before restart. Resume saved agents directly, or recreate shell-only entries.</p>
                  {bankedSessions.map(session => {
                    const recoverableAgent = isRecoverableAgent(session)
                    const resumeCommand = session.resumeCommand?.trim() || `/resume ${session.name}`
                    return (
                      <div key={`${session.unixUser || 'default'}:${session.name}`} className={`session-bank-item ${recoverableAgent ? 'session-bank-item-recoverable' : 'session-bank-item-shell'}`}>
                        <div className="session-bank-main">
                          <div className="session-bank-title-row">
                            <strong>{session.name}</strong>
                            <span className={`session-bank-badge ${recoverableAgent ? 'session-bank-badge-agent' : 'session-bank-badge-shell'}`}>
                              {recoverableAgent ? 'Recoverable agent' : 'Shell only'}
                            </span>
                          </div>
                          <span>{[session.id ? `id ${session.id}` : '', session.unixUser || 'default', `last seen ${new Date(session.lastSeen).toLocaleString()}`].filter(Boolean).join(' · ')}</span>
                          {recoverableAgent ? (
                            <>
                              <span>{[session.agentKind, session.agentSessionId].filter(Boolean).join(' · ')}</span>
                              <code>{resumeCommand}</code>
                            </>
                          ) : (
                            <>
                              <span>No agent resume metadata saved.</span>
                              <code>{resumeCommand}</code>
                            </>
                          )}
                        </div>
                        <div className="session-bank-actions">
                          {recoverableAgent && (
                            <button
                              type="button"
                              className="session-bank-resume"
                              onClick={() => void resumeBankedAgent(session)}
                              aria-label={`Resume agent for ${session.name}`}
                            >
                              Resume agent
                            </button>
                          )}
                          <button
                            type="button"
                            onClick={() => void copyResumeCommand(resumeCommand)}
                            aria-label={`Copy resume command for ${session.name}`}
                          >
                            Copy
                          </button>
                          <button
                            type="button"
                            onClick={() => void recreateBankedSession(session.name, session.unixUser)}
                            aria-label={`Recreate shell for ${session.name}`}
                          >
                            Recreate shell
                          </button>
                          <button
                            type="button"
                            className="session-bank-remove"
                            onClick={() => void removeBankedSession(session.name, session.unixUser)}
                            aria-label={`Remove ${session.name} from session bank`}
                          >
                            Remove
                          </button>
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </section>
          )}

          {!loading && !error && sortedGroups.length === 0 && bankedSessions.length === 0 && (
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
          protectedSessionNames={sessions.filter(s => s.persistent).map(s => s.name)}
          onConfirm={nukeAllSessions}
          onCancel={() => setShowNukeModal(false)}
        />
      )}
    </div>
  )
}

export default SessionPanel
